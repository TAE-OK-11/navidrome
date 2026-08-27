use std::io::{self, BufRead, BufReader, BufWriter, Cursor, Read, Write};

use anyhow::{Context, Result, bail};
use fast_image_resize as fir;
use image::codecs::jpeg::JpegEncoder;
use image::codecs::png::PngEncoder;
use image::codecs::gif::GifEncoder;
use image::{AnimationDecoder, ExtendedColorType, Frame, ImageEncoder, ImageFormat, ImageReader, Limits};
use serde::{Deserialize, Serialize};

const MAX_INPUT_BYTES: usize = 128 * 1024 * 1024;
const MAX_OUTPUT_BYTES: usize = 64 * 1024 * 1024;
const MAX_DIMENSION: u32 = 16_384;
const MAX_PIXELS: u64 = 40_000_000;

#[derive(Debug, Deserialize)]
struct ImageRequest {
    /// Single-image resize/fill payload length. Ignored when `mosaic` is set.
    #[serde(default)]
    input_size: usize,
    /// Concatenated mosaic tile payloads (1..=4), each filled to size/2.
    #[serde(default)]
    input_sizes: Vec<usize>,
    #[serde(default)]
    mosaic: bool,
    #[serde(default)]
    sniff: bool,
    #[serde(default)]
    size: u32,
    #[serde(default)]
    square: bool,
    #[serde(default)]
    fill: bool,
    #[serde(default)]
    animated_gif: bool,
    #[serde(default = "default_quality")]
    quality: u8,
    #[serde(default)]
    format: OutputFormat,
}

fn default_quality() -> u8 {
    75
}

#[derive(Debug)]
struct SniffAnimationFlags {
    animated_gif: bool,
    animated_webp: bool,
    animated_png: bool,
}

enum SniffResult {
    Animation(SniffAnimationFlags),
    Bytes(Vec<u8>),
}

#[derive(Debug, Deserialize, Default)]
#[serde(rename_all = "snake_case")]
enum OutputFormat {
    #[default]
    Jpeg,
    Png,
    Webp,
}

#[derive(Debug, Serialize)]
struct ImageResponse {
    ok: bool,
    size: usize,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    animated_gif: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    animated_webp: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    animated_png: Option<bool>,
}

pub fn run() -> Result<()> {
    let stdin = io::stdin();
    let stdout = io::stdout();
    let mut input = BufReader::with_capacity(128 * 1024, stdin.lock());
    let mut output = BufWriter::with_capacity(256 * 1024, stdout.lock());
    let mut header = String::with_capacity(512);

    loop {
        header.clear();
        if input
            .read_line(&mut header)
            .context("reading image request header")?
            == 0
        {
            return Ok(());
        }
        if header.trim().is_empty() {
            continue;
        }

        let request: ImageRequest = serde_json::from_str(&header)
            .context("invalid image request header; closing worker to resynchronize framing")?;
        validate_request(&request)
            .context("invalid image request; closing worker to resynchronize framing")?;

        let result = if request.sniff {
            let mut encoded = vec![0; request.input_size];
            input
                .read_exact(&mut encoded)
                .context("reading framed sniff payload")?;
            Ok(SniffResult::Animation(sniff_animation(&encoded)))
        } else if request.mosaic {
            let mut payloads = Vec::with_capacity(request.input_sizes.len());
            for &size in &request.input_sizes {
                let mut encoded = vec![0; size];
                input
                    .read_exact(&mut encoded)
                    .context("reading framed mosaic tile payload")?;
                payloads.push(encoded);
            }
            compose_mosaic(&payloads, &request).map(SniffResult::Bytes)
        } else {
            let mut encoded = vec![0; request.input_size];
            input
                .read_exact(&mut encoded)
                .context("reading framed image payload")?;
            resize(&encoded, &request).map(SniffResult::Bytes)
        };

        match result {
            Ok(SniffResult::Animation(flags)) => write_sniff(&mut output, flags)?,
            Ok(SniffResult::Bytes(resized)) => write_success(&mut output, &resized)?,
            Err(error) => write_error(&mut output, format!("{error:#}"))?,
        }
        output.flush().context("flushing image response")?;
    }
}

fn validate_request(request: &ImageRequest) -> Result<()> {
    if request.sniff {
        if request.input_size == 0 || request.input_size > MAX_INPUT_BYTES {
            bail!(
                "sniff input size {} is outside the allowed range 1..={MAX_INPUT_BYTES}",
                request.input_size
            );
        }
        return Ok(());
    }
    if request.mosaic {
        if request.input_sizes.is_empty() || request.input_sizes.len() > 4 {
            bail!(
                "mosaic requests require 1..=4 input_sizes, got {}",
                request.input_sizes.len()
            );
        }
        let mut total = 0usize;
        for &size in &request.input_sizes {
            if size == 0 || size > MAX_INPUT_BYTES {
                bail!("mosaic tile size {size} is outside the allowed range 1..={MAX_INPUT_BYTES}");
            }
            total = total
                .checked_add(size)
                .context("mosaic payload size overflow")?;
        }
        if total > MAX_INPUT_BYTES {
            bail!("combined mosaic payload {total} exceeds {MAX_INPUT_BYTES}");
        }
    } else if request.input_size == 0 || request.input_size > MAX_INPUT_BYTES {
        bail!(
            "input size {} is outside the allowed range 1..={MAX_INPUT_BYTES}",
            request.input_size
        );
    }
    if request.size == 0 || request.size > MAX_DIMENSION {
        bail!(
            "target size {} is outside the allowed range 1..={MAX_DIMENSION}",
            request.size
        );
    }
    if !(1..=100).contains(&request.quality) {
        bail!("quality must be between 1 and 100");
    }
    Ok(())
}

/// Fill each album cover to a size/2 tile and stitch 1 or 4 tiles into one PNG/JPEG/WebP.
/// Matches the Go playlist mosaic layout: one tile stays half-size; two/three are padded
/// to four by the caller before this runs.
fn compose_mosaic(payloads: &[Vec<u8>], request: &ImageRequest) -> Result<Vec<u8>> {
    let canvas = request.size;
    if canvas < 2 || canvas % 2 != 0 {
        bail!("mosaic canvas size must be an even value >= 2");
    }
    let tile = canvas / 2;

    let mut tiles_rgba = Vec::with_capacity(payloads.len());
    for payload in payloads {
        tiles_rgba.push(fill_to_tile_rgba(payload, tile)?);
    }

    let (pixels, width, height) = if tiles_rgba.len() == 1 {
        (tiles_rgba.remove(0), tile, tile)
    } else if tiles_rgba.len() == 4 {
        let mut canvas_pixels = vec![0u8; checked_rgba_len(canvas, canvas)?];
        let tile_stride = tile as usize * 4;
        let canvas_stride = canvas as usize * 4;
        let positions = [(0u32, 0u32), (tile, 0), (0, tile), (tile, tile)];
        for (tile_pixels, (origin_x, origin_y)) in tiles_rgba.iter().zip(positions) {
            for row in 0..tile as usize {
                let src = row * tile_stride;
                let dst = (row + origin_y as usize) * canvas_stride + origin_x as usize * 4;
                canvas_pixels[dst..dst + tile_stride]
                    .copy_from_slice(&tile_pixels[src..src + tile_stride]);
            }
        }
        (canvas_pixels, canvas, canvas)
    } else {
        bail!(
            "mosaic expects 1 or 4 tiles after caller padding, got {}",
            tiles_rgba.len()
        );
    };

    let result = encode(&pixels, width, height, request.quality, &request.format)?;
    if result.is_empty() || result.len() > MAX_OUTPUT_BYTES {
        bail!(
            "encoded mosaic size {} is outside the allowed range 1..={MAX_OUTPUT_BYTES}",
            result.len()
        );
    }
    Ok(result)
}

fn fill_to_tile_rgba(encoded: &[u8], tile: u32) -> Result<Vec<u8>> {
    let rgba = decode_rgba(encoded)?;
    let src_width = rgba.width();
    let src_height = rgba.height();
    validate_dimensions(src_width, src_height)?;
    if src_width == tile && src_height == tile {
        return Ok(rgba.into_raw());
    }
    let (crop_x, crop_y, crop_width, crop_height) = fill_crop(src_width, src_height, tile, tile);
    let cropped = crop_rgba(
        rgba.as_raw(),
        src_width,
        crop_x,
        crop_y,
        crop_width,
        crop_height,
    )?;
    let source =
        fir::images::Image::from_vec_u8(crop_width, crop_height, cropped, fir::PixelType::U8x4)
            .context("creating mosaic fill source")?;
    let mut resized = fir::images::Image::new(tile, tile, fir::PixelType::U8x4);
    let options = fir::ResizeOptions::new()
        .resize_alg(fir::ResizeAlg::Convolution(fir::FilterType::CatmullRom));
    fir::Resizer::new()
        .resize(&source, &mut resized, &options)
        .context("resizing mosaic tile")?;
    Ok(resized.into_vec())
}

fn decode_rgba(encoded: &[u8]) -> Result<image::RgbaImage> {
    let mut decoder = ImageReader::new(Cursor::new(encoded))
        .with_guessed_format()
        .context("detecting image format")?;
    let mut limits = Limits::default();
    limits.max_image_width = Some(MAX_DIMENSION);
    limits.max_image_height = Some(MAX_DIMENSION);
    limits.max_alloc = Some(MAX_PIXELS * 8);
    decoder.limits(limits);
    Ok(decoder.decode().context("decoding image")?.into_rgba8())
}

fn resize(encoded: &[u8], request: &ImageRequest) -> Result<Vec<u8>> {
    if request.animated_gif && is_animated_gif(encoded) {
        return resize_animated_gif(encoded, request);
    }
    let dimensions_reader = ImageReader::new(Cursor::new(encoded))
        .with_guessed_format()
        .context("detecting image format")?;
    let source_format = dimensions_reader
        .format()
        .context("detecting image format")?;
    let (src_width, src_height) = dimensions_reader
        .into_dimensions()
        .context("reading image dimensions")?;
    validate_dimensions(src_width, src_height)?;

    let original_size = src_width.max(src_height);
    let target_size = if request.fill {
        request.size
    } else {
        request.size.min(original_size)
    };
    let dimensions_match = if request.fill {
        src_width == target_size && src_height == target_size
    } else {
        target_size == original_size && !request.square
    };
    if dimensions_match && format_matches_request(source_format, &request.format) {
        return Ok(encoded.to_vec());
    }
    if !request.fill && target_size == original_size && !request.square {
        bail!("image does not require resizing");
    }
    if request.fill && src_width == target_size && src_height == target_size {
        bail!("image does not require resizing");
    }

    let mut decoder = ImageReader::new(Cursor::new(encoded))
        .with_guessed_format()
        .context("detecting image format")?;
    let mut limits = Limits::default();
    limits.max_image_width = Some(MAX_DIMENSION);
    limits.max_image_height = Some(MAX_DIMENSION);
    limits.max_alloc = Some(MAX_PIXELS * 8);
    decoder.limits(limits);
    let rgba = decoder.decode().context("decoding image")?.into_rgba8();

    let (pixels, output_width, output_height) = if request.fill {
        let (crop_x, crop_y, crop_width, crop_height) =
            fill_crop(src_width, src_height, target_size, target_size);
        let cropped = crop_rgba(
            rgba.as_raw(),
            src_width,
            crop_x,
            crop_y,
            crop_width,
            crop_height,
        )?;
        let source = fir::images::Image::from_vec_u8(
            crop_width,
            crop_height,
            cropped,
            fir::PixelType::U8x4,
        )
        .context("creating fill resize source")?;
        let mut resized = fir::images::Image::new(target_size, target_size, fir::PixelType::U8x4);
        let options = fir::ResizeOptions::new()
            .resize_alg(fir::ResizeAlg::Convolution(fir::FilterType::CatmullRom));
        fir::Resizer::new()
            .resize(&source, &mut resized, &options)
            .context("resizing filled image")?;
        (resized.into_vec(), target_size, target_size)
    } else {
        let (resized_width, resized_height) = fit_dimensions(src_width, src_height, target_size);
        let source = fir::images::Image::from_vec_u8(
            src_width,
            src_height,
            rgba.into_raw(),
            fir::PixelType::U8x4,
        )
        .context("creating resize source")?;
        let mut resized =
            fir::images::Image::new(resized_width, resized_height, fir::PixelType::U8x4);
        let options = fir::ResizeOptions::new()
            .resize_alg(fir::ResizeAlg::Convolution(fir::FilterType::CatmullRom));
        fir::Resizer::new()
            .resize(&source, &mut resized, &options)
            .context("resizing image")?;

        if request.square {
            let canvas_len = checked_rgba_len(target_size, target_size)?;
            let mut canvas = vec![0; canvas_len];
            let offset_x = (target_size - resized_width) / 2;
            let offset_y = (target_size - resized_height) / 2;
            let source_stride = resized_width as usize * 4;
            let destination_stride = target_size as usize * 4;
            for row in 0..resized_height as usize {
                let source_start = row * source_stride;
                let destination_start =
                    (row + offset_y as usize) * destination_stride + offset_x as usize * 4;
                canvas[destination_start..destination_start + source_stride]
                    .copy_from_slice(&resized.buffer()[source_start..source_start + source_stride]);
            }
            (canvas, target_size, target_size)
        } else {
            (resized.into_vec(), resized_width, resized_height)
        }
    };

    let result = encode(
        &pixels,
        output_width,
        output_height,
        request.quality,
        &request.format,
    )?;
    if result.is_empty() || result.len() > MAX_OUTPUT_BYTES {
        bail!(
            "encoded image size {} is outside the allowed range 1..={MAX_OUTPUT_BYTES}",
            result.len()
        );
    }
    Ok(result)
}

fn fill_crop(src_width: u32, src_height: u32, dst_width: u32, dst_height: u32) -> (u32, u32, u32, u32) {
    let src_aspect = f64::from(src_width) / f64::from(src_height);
    let dst_aspect = f64::from(dst_width) / f64::from(dst_height);
    if src_aspect > dst_aspect {
        // Match Go fillCenter truncation so playlist tiles stay pixel-aligned.
        let crop_width = ((f64::from(src_height) * dst_aspect) as u32).max(1);
        let crop_x = (src_width - crop_width) / 2;
        (crop_x, 0, crop_width, src_height)
    } else {
        let crop_height = ((f64::from(src_width) / dst_aspect) as u32).max(1);
        let crop_y = (src_height - crop_height) / 2;
        (0, crop_y, src_width, crop_height)
    }
}

fn crop_rgba(
    pixels: &[u8],
    src_width: u32,
    crop_x: u32,
    crop_y: u32,
    crop_width: u32,
    crop_height: u32,
) -> Result<Vec<u8>> {
    let mut cropped = vec![0; checked_rgba_len(crop_width, crop_height)?];
    let source_stride = src_width as usize * 4;
    let crop_stride = crop_width as usize * 4;
    let origin_x = crop_x as usize * 4;
    for row in 0..crop_height as usize {
        let source_start = (row + crop_y as usize) * source_stride + origin_x;
        let destination_start = row * crop_stride;
        cropped[destination_start..destination_start + crop_stride]
            .copy_from_slice(&pixels[source_start..source_start + crop_stride]);
    }
    Ok(cropped)
}

fn validate_dimensions(width: u32, height: u32) -> Result<()> {
    if width == 0 || height == 0 || width > MAX_DIMENSION || height > MAX_DIMENSION {
        bail!("image dimensions {width}x{height} exceed allowed limits");
    }
    if u64::from(width) * u64::from(height) > MAX_PIXELS {
        bail!("image dimensions {width}x{height} exceed the pixel budget {MAX_PIXELS}");
    }
    Ok(())
}

fn fit_dimensions(width: u32, height: u32, size: u32) -> (u32, u32) {
    if width >= height {
        (
            size,
            (u64::from(height) * u64::from(size) / u64::from(width)).max(1) as u32,
        )
    } else {
        (
            (u64::from(width) * u64::from(size) / u64::from(height)).max(1) as u32,
            size,
        )
    }
}

fn checked_rgba_len(width: u32, height: u32) -> Result<usize> {
    let bytes = u64::from(width)
        .checked_mul(u64::from(height))
        .and_then(|pixels| pixels.checked_mul(4))
        .context("image allocation overflow")?;
    usize::try_from(bytes).context("image allocation exceeds platform limits")
}

fn format_matches_request(source: ImageFormat, format: &OutputFormat) -> bool {
    matches!(
        (format, source),
        (OutputFormat::Jpeg, ImageFormat::Jpeg)
            | (OutputFormat::Png, ImageFormat::Png)
            | (OutputFormat::Webp, ImageFormat::WebP)
    )
}

fn encode(
    rgba: &[u8],
    width: u32,
    height: u32,
    quality: u8,
    format: &OutputFormat,
) -> Result<Vec<u8>> {
    let mut output = Vec::new();
    match format {
        OutputFormat::Jpeg => {
            let mut rgb = Vec::with_capacity(rgba.len() / 4 * 3);
            for pixel in rgba.chunks_exact(4) {
                rgb.extend_from_slice(&pixel[..3]);
            }
            JpegEncoder::new_with_quality(&mut output, quality)
                .write_image(&rgb, width, height, ExtendedColorType::Rgb8)
                .context("encoding JPEG")?;
        }
        OutputFormat::Png => {
            PngEncoder::new(&mut output)
                .write_image(rgba, width, height, ExtendedColorType::Rgba8)
                .context("encoding PNG")?;
        }
        OutputFormat::Webp => {
            let encoded = webp::Encoder::from_rgba(rgba, width, height)
                .encode_simple(false, f32::from(quality))
                .map_err(|error| anyhow::anyhow!("encoding WebP: {error:?}"))?;
            output.extend_from_slice(&encoded);
        }
    }
    Ok(output)
}

fn is_animated_gif(data: &[u8]) -> bool {
    data.starts_with(b"GIF") && data.iter().filter(|&&b| b == 0x2C).count() > 1
}

fn is_animated_webp(data: &[u8]) -> bool {
    data.starts_with(b"RIFF")
        && data.len() >= 12
        && &data[8..12] == b"WEBP"
        && data.windows(4).any(|window| window == b"ANMF")
}

fn is_animated_png(data: &[u8]) -> bool {
    data.starts_with(&[0x89, b'P', b'N', b'G', b'\r', b'\n', 0x1A, b'\n'])
        && data.windows(4).any(|window| window == b"acTL")
}

fn sniff_animation(data: &[u8]) -> SniffAnimationFlags {
    SniffAnimationFlags {
        animated_gif: is_animated_gif(data),
        animated_webp: is_animated_webp(data),
        animated_png: is_animated_png(data),
    }
}

fn write_sniff(output: &mut impl Write, flags: SniffAnimationFlags) -> Result<()> {
    serde_json::to_writer(
        &mut *output,
        &ImageResponse {
            ok: true,
            size: 0,
            error: None,
            animated_gif: Some(flags.animated_gif),
            animated_webp: Some(flags.animated_webp),
            animated_png: Some(flags.animated_png),
        },
    )?;
    output.write_all(b"\n")?;
    Ok(())
}

fn resize_animated_gif(encoded: &[u8], request: &ImageRequest) -> Result<Vec<u8>> {
    use image::codecs::gif::GifDecoder;

    let decoder = GifDecoder::new(Cursor::new(encoded)).context("decoding animated gif")?;
    let mut resized_frames = Vec::new();
    for frame in decoder.into_frames() {
        let frame = frame.context("reading gif frame")?;
        let delay = frame.delay();
        let rgba = frame.into_buffer();
        let (src_width, src_height) = (rgba.width(), rgba.height());
        validate_dimensions(src_width, src_height)?;
        let target_size = request.size.min(src_width.max(src_height));
        let (resized_width, resized_height) = fit_dimensions(src_width, src_height, target_size);
        let source = fir::images::Image::from_vec_u8(
            src_width,
            src_height,
            rgba.into_raw(),
            fir::PixelType::U8x4,
        )
        .context("creating animated gif resize source")?;
        let mut resized =
            fir::images::Image::new(resized_width, resized_height, fir::PixelType::U8x4);
        let options = fir::ResizeOptions::new()
            .resize_alg(fir::ResizeAlg::Convolution(fir::FilterType::CatmullRom));
        fir::Resizer::new()
            .resize(&source, &mut resized, &options)
            .context("resizing animated gif frame")?;
        let buffer = image::RgbaImage::from_raw(resized_width, resized_height, resized.into_vec())
            .context("building animated gif frame buffer")?;
        resized_frames.push(Frame::from_parts(buffer, 0, 0, delay));
    }
    if resized_frames.is_empty() {
        bail!("animated gif contains no frames");
    }
    let mut output = Vec::new();
    {
        let mut encoder = GifEncoder::new(&mut output);
        for frame in resized_frames {
            encoder
                .encode_frame(frame)
                .context("encoding animated gif frame")?;
        }
    }
    if output.is_empty() || output.len() > MAX_OUTPUT_BYTES {
        bail!(
            "encoded animated gif size {} is outside the allowed range 1..={MAX_OUTPUT_BYTES}",
            output.len()
        );
    }
    Ok(output)
}

fn write_success(output: &mut impl Write, image: &[u8]) -> Result<()> {
    serde_json::to_writer(
        &mut *output,
        &ImageResponse {
            ok: true,
            size: image.len(),
            error: None,
            animated_gif: None,
            animated_webp: None,
            animated_png: None,
        },
    )?;
    output.write_all(b"\n")?;
    output.write_all(image)?;
    Ok(())
}

fn write_error(output: &mut impl Write, error: String) -> Result<()> {
    serde_json::to_writer(
        &mut *output,
        &ImageResponse {
            ok: false,
            size: 0,
            error: Some(error),
            animated_gif: None,
            animated_webp: None,
            animated_png: None,
        },
    )?;
    output.write_all(b"\n")?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use image::{Rgba, RgbaImage};

    fn source_png(width: u32, height: u32) -> Vec<u8> {
        let image = RgbaImage::from_pixel(width, height, Rgba([30, 90, 180, 255]));
        let mut output = Vec::new();
        PngEncoder::new(&mut output)
            .write_image(image.as_raw(), width, height, ExtendedColorType::Rgba8)
            .unwrap();
        output
    }

    #[test]
    fn passthrough_returns_original_when_dimensions_and_format_match() {
        let input = source_png(120, 120);
        let output = resize(
            &input,
            &ImageRequest {
                input_size: input.len(),
                input_sizes: Vec::new(),
                mosaic: false,
                sniff: false,
                size: 120,
                square: false,
                fill: false,
                animated_gif: false,
                quality: 80,
                format: OutputFormat::Png,
            },
        )
        .unwrap();
        assert_eq!(output, input);
    }

    #[test]
    fn resizes_and_centers_on_square_canvas() {
        let input = source_png(80, 40);
        let output = resize(
            &input,
            &ImageRequest {
                input_size: input.len(),
                input_sizes: Vec::new(),
                mosaic: false,
                sniff: false,
                size: 20,
                square: true,
                fill: false,
                animated_gif: false,
                quality: 80,
                format: OutputFormat::Png,
            },
        )
        .unwrap();
        let decoded = image::load_from_memory(&output).unwrap();
        assert_eq!((decoded.width(), decoded.height()), (20, 20));
        assert_eq!(decoded.to_rgba8().get_pixel(0, 0).0[3], 0);
    }

    #[test]
    fn fill_crops_center_and_scales_to_exact_square() {
        let input = source_png(80, 40);
        let output = resize(
            &input,
            &ImageRequest {
                input_size: input.len(),
                input_sizes: Vec::new(),
                mosaic: false,
                sniff: false,
                size: 20,
                square: false,
                fill: true,
                animated_gif: false,
                quality: 80,
                format: OutputFormat::Png,
            },
        )
        .unwrap();
        let decoded = image::load_from_memory(&output).unwrap();
        assert_eq!((decoded.width(), decoded.height()), (20, 20));
        // Fill crops the wider source, so the canvas is fully opaque.
        assert_eq!(decoded.to_rgba8().get_pixel(0, 0).0[3], 255);
        assert_eq!(fill_crop(80, 40, 20, 20), (20, 0, 40, 40));
    }

    #[test]
    fn compose_mosaic_stitches_four_filled_tiles() {
        let tiles = [
            source_png(80, 40),
            source_png(40, 80),
            source_png(60, 60),
            source_png(100, 50),
        ];
        let output = compose_mosaic(
            &tiles,
            &ImageRequest {
                input_size: 0,
                input_sizes: tiles.iter().map(Vec::len).collect(),
                mosaic: true,
                sniff: false,
                size: 40,
                square: false,
                fill: false,
                animated_gif: false,
                quality: 80,
                format: OutputFormat::Png,
            },
        )
        .unwrap();
        let decoded = image::load_from_memory(&output).unwrap();
        assert_eq!((decoded.width(), decoded.height()), (40, 40));
        assert_eq!(decoded.to_rgba8().get_pixel(0, 0).0[3], 255);
        assert_eq!(decoded.to_rgba8().get_pixel(39, 39).0[3], 255);
    }

    #[test]
    fn compose_mosaic_single_tile_stays_half_canvas() {
        let tile = source_png(80, 40);
        let output = compose_mosaic(
            &[tile.clone()],
            &ImageRequest {
                input_size: 0,
                input_sizes: vec![tile.len()],
                mosaic: true,
                sniff: false,
                size: 40,
                square: false,
                fill: false,
                animated_gif: false,
                quality: 80,
                format: OutputFormat::Png,
            },
        )
        .unwrap();
        let decoded = image::load_from_memory(&output).unwrap();
        assert_eq!((decoded.width(), decoded.height()), (20, 20));
    }

    #[test]
    fn rejects_images_over_pixel_budget_before_decode() {
        assert!(validate_dimensions(10_000, 5_000).is_err());
        assert!(validate_dimensions(4_000, 4_000).is_ok());
    }

    #[test]
    fn preserves_aspect_ratio_without_zero_sized_edges() {
        assert_eq!(fit_dimensions(4_000, 1, 1), (1, 1));
        assert_eq!(fit_dimensions(40, 80, 20), (10, 20));
    }
}
