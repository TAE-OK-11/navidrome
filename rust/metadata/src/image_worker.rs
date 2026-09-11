use std::fs;
use std::io::{self, BufRead, BufReader, BufWriter, Cursor, Read, Write};
use std::path::Path;
use std::process::{Child, Command, Stdio};
use std::time::{Duration, Instant};

use anyhow::{Context, Result, bail};
use fast_image_resize as fir;
use image::codecs::gif::GifEncoder;
use image::codecs::jpeg::JpegEncoder;
use image::codecs::png::PngEncoder;
use image::{
    AnimationDecoder, ExtendedColorType, Frame, ImageEncoder, ImageFormat, ImageReader, Limits,
};
use serde::{Deserialize, Serialize};

const MAX_INPUT_BYTES: usize = 128 * 1024 * 1024;
const MAX_OUTPUT_BYTES: usize = 64 * 1024 * 1024;
const MAX_DIMENSION: u32 = 16_384;
const MAX_PIXELS: u64 = 40_000_000;

#[derive(Debug, Deserialize)]
pub struct ImageRequest {
    /// Single-image resize/fill payload length. Ignored when `mosaic` is set.
    #[serde(default)]
    pub input_size: usize,
    /// Concatenated mosaic tile payloads (1..=4), each filled to size/2.
    #[serde(default)]
    pub input_sizes: Vec<usize>,
    #[serde(default)]
    pub mosaic: bool,
    #[serde(default)]
    pub sniff: bool,
    /// Header-only inspect: return width/height/format without resize/encode.
    #[serde(default)]
    pub validate: bool,
    #[serde(default)]
    pub size: u32,
    #[serde(default)]
    pub square: bool,
    #[serde(default)]
    pub fill: bool,
    #[serde(default)]
    pub animated_gif: bool,
    #[serde(default)]
    pub animated_webp: bool,
    #[serde(default)]
    pub animated_png: bool,
    #[serde(default = "default_quality")]
    pub quality: u8,
    #[serde(default)]
    pub format: OutputFormat,
    /// When set, the worker reads image bytes from this local file path instead
    /// of the framed stdin payload (`input_size` must be 0).
    #[serde(default)]
    pub path: Option<String>,
}

fn default_quality() -> u8 {
    75
}

#[derive(Debug)]
pub struct SniffAnimationFlags {
    pub animated_gif: bool,
    pub animated_webp: bool,
    pub animated_png: bool,
}

#[derive(Debug)]
pub struct ValidateImageInfo {
    pub width: u32,
    pub height: u32,
    pub format: String,
}

enum SniffResult {
    Animation(SniffAnimationFlags),
    Validate(ValidateImageInfo),
    Bytes(Vec<u8>),
}

#[derive(Debug, Deserialize, Default, Clone)]
#[serde(rename_all = "snake_case")]
pub enum OutputFormat {
    #[default]
    Jpeg,
    Png,
    Webp,
}

impl OutputFormat {
    pub fn parse(value: &str) -> Self {
        match value.trim().to_ascii_lowercase().as_str() {
            "png" => Self::Png,
            "webp" => Self::Webp,
            _ => Self::Jpeg,
        }
    }
}

pub enum ImageOutcome {
    Bytes(Vec<u8>),
    Sniff(SniffAnimationFlags),
    Validate(ValidateImageInfo),
}

pub fn process(mut request: ImageRequest, payloads: Vec<Vec<u8>>) -> Result<ImageOutcome> {
    if request.mosaic {
        request.input_sizes = payloads.iter().map(Vec::len).collect();
    } else if !uses_path(&request) && request.input_size == 0 {
        request.input_size = payloads.first().map(Vec::len).unwrap_or(0);
    }
    validate_request(&request)?;
    if request.sniff {
        let encoded = if uses_path(&request) {
            read_image_file(request.path.as_deref().unwrap_or_default())?
        } else {
            payloads.into_iter().next().unwrap_or_default()
        };
        return Ok(ImageOutcome::Sniff(sniff_animation(&encoded)));
    }
    if request.validate {
        let encoded = if uses_path(&request) {
            read_image_file(request.path.as_deref().unwrap_or_default())?
        } else {
            payloads.into_iter().next().unwrap_or_default()
        };
        return Ok(ImageOutcome::Validate(validate_image(&encoded)?));
    }
    if request.mosaic {
        return Ok(ImageOutcome::Bytes(compose_mosaic(&payloads, &request)?));
    }
    let encoded = if uses_path(&request) {
        read_image_file(request.path.as_deref().unwrap_or_default())?
    } else {
        payloads.into_iter().next().unwrap_or_default()
    };
    Ok(ImageOutcome::Bytes(resize(&encoded, &request)?))
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
    #[serde(skip_serializing_if = "Option::is_none")]
    width: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    height: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    format: Option<String>,
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
            let encoded = read_input_bytes(&request, &mut input)?;
            Ok(SniffResult::Animation(sniff_animation(&encoded)))
        } else if request.validate {
            let encoded = read_input_bytes(&request, &mut input)?;
            validate_image(&encoded).map(SniffResult::Validate)
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
            let encoded = read_input_bytes(&request, &mut input)?;
            resize(&encoded, &request).map(SniffResult::Bytes)
        };

        match result {
            Ok(SniffResult::Animation(flags)) => write_sniff(&mut output, flags)?,
            Ok(SniffResult::Validate(info)) => write_validate(&mut output, info)?,
            Ok(SniffResult::Bytes(resized)) => write_success(&mut output, &resized)?,
            Err(error) => write_error(&mut output, format!("{error:#}"))?,
        }
        output.flush().context("flushing image response")?;
    }
}

fn uses_path(request: &ImageRequest) -> bool {
    request
        .path
        .as_ref()
        .is_some_and(|path| !path.trim().is_empty())
}

fn read_input_bytes(request: &ImageRequest, input: &mut impl Read) -> Result<Vec<u8>> {
    if let Some(path) = request.path.as_ref().filter(|path| !path.trim().is_empty()) {
        return read_image_file(path);
    }
    let mut encoded = vec![0; request.input_size];
    input
        .read_exact(&mut encoded)
        .context("reading framed image payload")?;
    Ok(encoded)
}

fn read_image_file(path: &str) -> Result<Vec<u8>> {
    let path = Path::new(path);
    // Inspect and read the same opened file. A replacement path or file growth
    // after metadata was inspected must not bypass the payload budget.
    let file =
        fs::File::open(path).with_context(|| format!("opening image file {}", path.display()))?;
    let metadata = file
        .metadata()
        .with_context(|| format!("reading image file metadata {}", path.display()))?;
    if !metadata.is_file() {
        bail!("image path {} is not a regular file", path.display());
    }
    let len = metadata.len();
    if len == 0 || len > MAX_INPUT_BYTES as u64 {
        bail!("image file size {len} is outside the allowed range 1..={MAX_INPUT_BYTES}");
    }
    let encoded = read_bounded(file, MAX_INPUT_BYTES, len as usize)
        .with_context(|| format!("reading image file {}", path.display()))?;
    if encoded.is_empty() {
        bail!("image file {} is empty", path.display());
    }
    Ok(encoded)
}

fn validate_request(request: &ImageRequest) -> Result<()> {
    if request.mosaic && uses_path(request) {
        bail!("mosaic requests cannot use path mode");
    }
    if request.sniff && request.validate {
        bail!("sniff and validate are mutually exclusive");
    }
    if request.sniff || request.validate {
        let kind = if request.sniff { "sniff" } else { "validate" };
        if uses_path(request) {
            return Ok(());
        }
        if request.input_size == 0 || request.input_size > MAX_INPUT_BYTES {
            bail!(
                "{kind} input size {} is outside the allowed range 1..={MAX_INPUT_BYTES}",
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
    } else if uses_path(request) {
        // Path mode reads the file in Rust; no stdin payload is required.
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
    if request.animated_webp && is_animated_webp(encoded) {
        return resize_animated_webp(encoded, request);
    }
    if request.animated_png && is_animated_png(encoded) {
        return resize_animated_png(encoded, request);
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
        let source =
            fir::images::Image::from_vec_u8(crop_width, crop_height, cropped, fir::PixelType::U8x4)
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

fn fill_crop(
    src_width: u32,
    src_height: u32,
    dst_width: u32,
    dst_height: u32,
) -> (u32, u32, u32, u32) {
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

fn validate_image(encoded: &[u8]) -> Result<ValidateImageInfo> {
    if encoded.is_empty() || encoded.len() > MAX_INPUT_BYTES {
        bail!(
            "validate input size {} is outside the allowed range 1..={MAX_INPUT_BYTES}",
            encoded.len()
        );
    }
    let reader = ImageReader::new(Cursor::new(encoded))
        .with_guessed_format()
        .context("detecting image format")?;
    let format = reader.format().context("detecting image format")?;
    let format_name = upload_format_name(format)?;
    let (width, height) = reader
        .into_dimensions()
        .context("reading image dimensions")?;
    validate_dimensions(width, height)?;
    Ok(ValidateImageInfo {
        width,
        height,
        format: format_name.to_owned(),
    })
}

fn upload_format_name(format: ImageFormat) -> Result<&'static str> {
    // Match Go nativeapi registrations: jpeg/png/gif/webp only.
    Ok(match format {
        ImageFormat::Jpeg => "jpeg",
        ImageFormat::Png => "png",
        ImageFormat::Gif => "gif",
        ImageFormat::WebP => "webp",
        other => bail!("unsupported image format for upload validation: {other:?}"),
    })
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
            width: None,
            height: None,
            format: None,
        },
    )?;
    output.write_all(b"\n")?;
    Ok(())
}

fn write_validate(output: &mut impl Write, info: ValidateImageInfo) -> Result<()> {
    serde_json::to_writer(
        &mut *output,
        &ImageResponse {
            ok: true,
            size: 0,
            error: None,
            animated_gif: None,
            animated_webp: None,
            animated_png: None,
            width: Some(info.width),
            height: Some(info.height),
            format: Some(info.format),
        },
    )?;
    output.write_all(b"\n")?;
    Ok(())
}

fn resize_animated_gif(encoded: &[u8], request: &ImageRequest) -> Result<Vec<u8>> {
    use image::ImageDecoder;
    use image::codecs::gif::GifDecoder;

    let mut decoder = GifDecoder::new(Cursor::new(encoded)).context("decoding animated gif")?;
    let (width, height) = decoder.dimensions();
    validate_dimensions(width, height)?;
    let mut limits = Limits::default();
    limits.max_image_width = Some(MAX_DIMENSION);
    limits.max_image_height = Some(MAX_DIMENSION);
    limits.max_alloc = Some(MAX_PIXELS * 8);
    decoder
        .set_limits(limits)
        .context("limiting animated gif decoder")?;
    let mut output = LimitedImageBuffer {
        bytes: Vec::new(),
        limit: MAX_OUTPUT_BYTES,
        exceeded: false,
    };
    let mut encoder = GifEncoder::new(&mut output);
    let mut frame_count = 0;
    let mut resizer = fir::Resizer::new();
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
        resizer
            .resize(&source, &mut resized, &options)
            .context("resizing animated gif frame")?;
        let buffer = image::RgbaImage::from_raw(resized_width, resized_height, resized.into_vec())
            .context("building animated gif frame buffer")?;
        // Encode each resized frame immediately instead of retaining every
        // frame's RGBA buffer until the entire animation has been decoded.
        encoder
            .encode_frame(Frame::from_parts(buffer, 0, 0, delay))
            .context("encoding animated gif frame")?;
        frame_count += 1;
    }
    drop(encoder);
    if output.exceeded {
        bail!("encoded animated gif exceeds output byte limit");
    }
    if frame_count == 0 {
        bail!("animated gif contains no frames");
    }
    Ok(output.bytes)
}

struct LimitedImageBuffer {
    bytes: Vec<u8>,
    limit: usize,
    // The underlying GIF encoder finalizes in Drop, which cannot report errors.
    exceeded: bool,
}

impl Write for LimitedImageBuffer {
    fn write(&mut self, bytes: &[u8]) -> io::Result<usize> {
        if bytes.len() > self.limit.saturating_sub(self.bytes.len()) {
            self.exceeded = true;
            return Err(io::Error::other("encoded image exceeds output byte limit"));
        }
        self.bytes.extend_from_slice(bytes);
        Ok(bytes.len())
    }

    fn flush(&mut self) -> io::Result<()> {
        Ok(())
    }
}

fn resize_animated_webp(encoded: &[u8], request: &ImageRequest) -> Result<Vec<u8>> {
    let scale = format!(
        "scale='min({0},iw)':'min({0},ih)':force_original_aspect_ratio=decrease",
        request.size
    );
    let quality = request.quality.to_string();
    let mut command = Command::new("ffmpeg");
    command.args([
        "-hide_banner",
        "-loglevel",
        "error",
        "-i",
        "pipe:0",
        "-vf",
        &scale,
        "-loop",
        "0",
        "-c:v",
        "libwebp_anim",
        "-quality",
        &quality,
        "-f",
        "webp",
        "pipe:1",
    ]);
    run_image_command(
        &mut command,
        encoded,
        MAX_OUTPUT_BYTES,
        Duration::from_secs(120),
    )
    .context("resizing animated WebP with ffmpeg")
}

fn resize_animated_png(encoded: &[u8], request: &ImageRequest) -> Result<Vec<u8>> {
    let scale = format!(
        "scale='min({0},iw)':'min({0},ih)':force_original_aspect_ratio=decrease",
        request.size
    );
    let mut command = Command::new("ffmpeg");
    command.args([
        "-hide_banner",
        "-loglevel",
        "error",
        "-i",
        "pipe:0",
        "-vf",
        &scale,
        "-plays",
        "0",
        "-f",
        "apng",
        "pipe:1",
    ]);
    run_image_command(
        &mut command,
        encoded,
        MAX_OUTPUT_BYTES,
        Duration::from_secs(120),
    )
    .context("resizing animated PNG with ffmpeg")
}

// A guard also cleans up on thread creation failure or an early I/O error.
// It lives inside the scope so the child exits before scoped threads are joined.
struct ImageChild(Child);

impl Drop for ImageChild {
    fn drop(&mut self) {
        let _ = self.0.kill();
        let _ = self.0.wait();
    }
}

fn read_bounded(reader: impl Read, limit: usize, capacity_hint: usize) -> io::Result<Vec<u8>> {
    let mut bytes = Vec::with_capacity(capacity_hint.min(limit));
    reader.take(limit as u64 + 1).read_to_end(&mut bytes)?;
    if bytes.len() > limit {
        return Err(io::Error::other("image stream exceeds byte limit"));
    }
    Ok(bytes)
}

fn read_diagnostics(mut reader: impl Read) -> io::Result<Vec<u8>> {
    // Retain a useful error message while continuing to drain the pipe, even
    // when malformed inputs cause ffmpeg to print many repeated diagnostics.
    let mut bytes = Vec::new();
    reader.by_ref().take(64 * 1024).read_to_end(&mut bytes)?;
    io::copy(&mut reader, &mut io::sink())?;
    Ok(bytes)
}

fn run_image_command(
    command: &mut Command,
    encoded: &[u8],
    output_limit: usize,
    timeout: Duration,
) -> Result<Vec<u8>> {
    std::thread::scope(|scope| {
        let mut child = ImageChild(
            command
                .stdin(Stdio::piped())
                .stdout(Stdio::piped())
                .stderr(Stdio::piped())
                .spawn()
                .context("starting image encoder")?,
        );
        let mut stdin = child.0.stdin.take().context("encoder stdin unavailable")?;
        let stdout = child
            .0
            .stdout
            .take()
            .context("encoder stdout unavailable")?;
        let stderr = child
            .0
            .stderr
            .take()
            .context("encoder stderr unavailable")?;
        let (sender, receiver) = std::sync::mpsc::channel();
        let input_sender = sender.clone();
        std::thread::Builder::new()
            .name("image-stdin".into())
            .spawn_scoped(scope, move || {
                let result = stdin.write_all(encoded).map(|()| Vec::new());
                // Close stdin before publishing completion so the encoder sees EOF.
                drop(stdin);
                let _ = input_sender.send((0, result));
            })
            .context("starting image input writer")?;
        let output_sender = sender.clone();
        std::thread::Builder::new()
            .name("image-stdout".into())
            .spawn_scoped(scope, move || {
                let _ = output_sender.send((1, read_bounded(stdout, output_limit, 0)));
            })
            .context("starting image output reader")?;
        std::thread::Builder::new()
            .name("image-stderr".into())
            .spawn_scoped(scope, move || {
                let _ = sender.send((2, read_diagnostics(stderr)));
            })
            .context("starting image diagnostic reader")?;

        let deadline = Instant::now() + timeout;
        let mut output = Vec::new();
        let mut diagnostics = Vec::new();
        let mut io_error = None;
        for _ in 0..3 {
            let (pipe, result) = receiver
                .recv_timeout(deadline.saturating_duration_since(Instant::now()))
                .context("image encoder timed out or a pipe worker stopped")?;
            match result {
                Ok(bytes) if pipe == 1 => output = bytes,
                Ok(bytes) if pipe == 2 => diagnostics = bytes,
                Ok(_) => {}
                Err(error) => {
                    // In particular, stop encoding immediately at the output
                    // limit so the stdin writer cannot remain blocked forever.
                    let _ = child.0.kill();
                    if io_error.is_none() || pipe == 1 {
                        io_error = Some(error);
                    }
                }
            }
        }
        // A process can close all three pipes but continue running. Poll its
        // exit using the same deadline instead of blocking in wait indefinitely.
        let status = loop {
            if let Some(status) = child.0.try_wait().context("waiting for image encoder")? {
                break status;
            }
            if Instant::now() >= deadline {
                bail!("image encoder timed out");
            }
            std::thread::sleep(Duration::from_millis(10));
        };
        if let Some(error) = io_error {
            return Err(error).with_context(|| {
                format!(
                    "transferring image encoder data: {}",
                    String::from_utf8_lossy(&diagnostics)
                )
            });
        }
        if !status.success() {
            bail!(
                "image encoder failed: {}",
                String::from_utf8_lossy(&diagnostics)
            );
        }
        if output.is_empty() {
            bail!("image encoder returned empty output");
        }
        Ok(output)
    })
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
            width: None,
            height: None,
            format: None,
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
            width: None,
            height: None,
            format: None,
        },
    )?;
    output.write_all(b"\n")?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use image::{Rgba, RgbaImage};

    #[test]
    fn bounded_image_reader_rejects_growth_beyond_limit() {
        let mut input = Cursor::new(b"123456789".as_slice());
        assert!(read_bounded(&mut input, 4, 2).is_err());
        assert_eq!(input.position(), 5, "must stop after the limit probe");
        assert_eq!(read_bounded(Cursor::new(b"1234"), 4, 4).unwrap(), b"1234");
    }

    #[test]
    fn encoded_image_writer_never_exceeds_limit() {
        let mut output = LimitedImageBuffer {
            bytes: Vec::new(),
            limit: 4,
            exceeded: false,
        };
        output.write_all(b"1234").unwrap();
        assert!(output.write_all(b"5").is_err());
        assert_eq!(output.bytes, b"1234");
    }

    #[test]
    fn diagnostics_are_capped_but_pipe_is_fully_drained() {
        let mut input = Cursor::new(vec![b'x'; 128 * 1024]);
        assert_eq!(read_diagnostics(&mut input).unwrap().len(), 64 * 1024);
        assert_eq!(input.position(), 128 * 1024);
    }

    #[cfg(unix)]
    #[test]
    fn image_encoder_drains_output_while_feeding_large_input() {
        // Output larger than a pipe must be consumed before this child begins
        // reading stdin. Writing all stdin before draining stdout deadlocks.
        let mut command = Command::new("sh");
        command.args(["-c", "head -c 131072 /dev/zero; exec cat"]);
        let input = vec![b'x'; 256 * 1024];
        let output =
            run_image_command(&mut command, &input, 512 * 1024, Duration::from_secs(5)).unwrap();
        assert_eq!(&output[..128 * 1024], vec![0; 128 * 1024]);
        assert_eq!(&output[128 * 1024..], input);
    }

    #[cfg(unix)]
    #[test]
    fn image_encoder_stops_when_output_exceeds_limit() {
        let mut command = Command::new("cat");
        let error = run_image_command(
            &mut command,
            &vec![b'x'; 256 * 1024],
            1024,
            Duration::from_secs(5),
        )
        .unwrap_err();
        assert!(format!("{error:#}").contains("byte limit"));
    }

    #[cfg(unix)]
    #[test]
    fn image_encoder_preserves_failure_diagnostics() {
        let mut command = Command::new("sh");
        command.args(["-c", "printf 'invalid animation' >&2; exit 7"]);
        let error = run_image_command(&mut command, &[], 1024, Duration::from_secs(5)).unwrap_err();
        assert!(format!("{error:#}").contains("invalid animation"));
    }

    #[cfg(unix)]
    #[test]
    fn image_encoder_kills_and_reaps_hung_child() {
        for close_pipes in [false, true] {
            let mut command = Command::new("sh");
            command.args([
                "-c",
                if close_pipes {
                    "exec 0<&- 1>&- 2>&-; exec sleep 60"
                } else {
                    "exec sleep 60"
                },
            ]);
            let started = Instant::now();
            let error =
                run_image_command(&mut command, &[], 1024, Duration::from_millis(100)).unwrap_err();
            assert!(format!("{error:#}").contains("timed out"));
            assert!(started.elapsed() < Duration::from_secs(5));
        }
    }

    #[test]
    fn animated_gif_rejects_oversized_canvas_before_decoding_frames() {
        let mut input = Vec::new();
        {
            let mut encoder = GifEncoder::new(&mut input);
            encoder
                .encode_frame(Frame::new(RgbaImage::new(1, 1)))
                .unwrap();
        }
        // Expand only the logical screen header, leaving a tiny valid frame.
        input[6..8].copy_from_slice(&u16::MAX.to_le_bytes());
        input[8..10].copy_from_slice(&u16::MAX.to_le_bytes());
        let request: ImageRequest = serde_json::from_str(r#"{"size":4}"#).unwrap();
        let error = resize_animated_gif(&input, &request).unwrap_err();
        assert!(format!("{error:#}").contains("exceed allowed limits"));
    }

    #[test]
    fn animated_gif_preserves_frames_and_delays_while_resizing() {
        use image::codecs::gif::GifDecoder;
        let mut input = Vec::new();
        {
            let mut encoder = GifEncoder::new(&mut input);
            for (color, delay) in [([255, 0, 0, 255], 100), ([0, 0, 255, 255], 250)] {
                encoder
                    .encode_frame(Frame::from_parts(
                        RgbaImage::from_pixel(8, 4, Rgba(color)),
                        0,
                        0,
                        image::Delay::from_numer_denom_ms(delay, 1),
                    ))
                    .unwrap();
            }
        }
        let request: ImageRequest =
            serde_json::from_str(r#"{"size":4,"animated_gif":true}"#).unwrap();
        let output = resize_animated_gif(&input, &request).unwrap();
        let frames = GifDecoder::new(Cursor::new(output))
            .unwrap()
            .into_frames()
            .collect_frames()
            .unwrap();
        assert_eq!(frames.len(), 2);
        for (frame, (color, delay)) in frames
            .iter()
            .zip([([255, 0, 0, 255], 100), ([0, 0, 255, 255], 250)])
        {
            assert_eq!(frame.buffer().dimensions(), (4, 2));
            assert_eq!(frame.buffer().get_pixel(0, 0).0, color);
            assert_eq!(frame.delay().numer_denom_ms(), (delay, 1));
        }
    }

    fn source_png(width: u32, height: u32) -> Vec<u8> {
        let image = RgbaImage::from_pixel(width, height, Rgba([30, 90, 180, 255]));
        let mut output = Vec::new();
        PngEncoder::new(&mut output)
            .write_image(image.as_raw(), width, height, ExtendedColorType::Rgba8)
            .unwrap();
        output
    }

    #[test]
    fn reads_image_from_path() {
        let dir = std::env::temp_dir().join(format!("navidrome-image-path-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        let path = dir.join("source.png");
        let input = source_png(120, 120);
        std::fs::write(&path, &input).unwrap();

        let output = resize(
            &std::fs::read(&path).unwrap(),
            &ImageRequest {
                input_size: input.len(),
                input_sizes: Vec::new(),
                mosaic: false,
                sniff: false,
                validate: false,
                size: 60,
                square: false,
                fill: false,
                animated_gif: false,
                animated_webp: false,
                animated_png: false,
                quality: 80,
                format: OutputFormat::Png,
                path: None,
            },
        )
        .unwrap();
        let decoded = image::load_from_memory(&output).unwrap();
        assert_eq!((decoded.width(), decoded.height()), (60, 60));

        let output_from_path = {
            let encoded = read_image_file(path.to_str().unwrap()).unwrap();
            resize(
                &encoded,
                &ImageRequest {
                    input_size: 0,
                    input_sizes: Vec::new(),
                    mosaic: false,
                    sniff: false,
                    validate: false,
                    size: 60,
                    square: false,
                    fill: false,
                    animated_gif: false,
                    animated_webp: false,
                    animated_png: false,
                    quality: 80,
                    format: OutputFormat::Png,
                    path: Some(path.to_str().unwrap().to_owned()),
                },
            )
            .unwrap()
        };
        let decoded = image::load_from_memory(&output_from_path).unwrap();
        assert_eq!((decoded.width(), decoded.height()), (60, 60));

        let _ = std::fs::remove_dir_all(&dir);
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
                validate: false,
                size: 120,
                square: false,
                fill: false,
                animated_gif: false,
                animated_webp: false,
                animated_png: false,
                quality: 80,
                format: OutputFormat::Png,
                path: None,
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
                validate: false,
                size: 20,
                square: true,
                fill: false,
                animated_gif: false,
                animated_webp: false,
                animated_png: false,
                quality: 80,
                format: OutputFormat::Png,
                path: None,
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
                validate: false,
                size: 20,
                square: false,
                fill: true,
                animated_gif: false,
                animated_webp: false,
                animated_png: false,
                quality: 80,
                format: OutputFormat::Png,
                path: None,
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
                validate: false,
                size: 40,
                square: false,
                fill: false,
                animated_gif: false,
                animated_webp: false,
                animated_png: false,
                quality: 80,
                format: OutputFormat::Png,
                path: None,
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
                validate: false,
                size: 40,
                square: false,
                fill: false,
                animated_gif: false,
                animated_webp: false,
                animated_png: false,
                quality: 80,
                format: OutputFormat::Png,
                path: None,
            },
        )
        .unwrap();
        let decoded = image::load_from_memory(&output).unwrap();
        assert_eq!((decoded.width(), decoded.height()), (20, 20));
    }

    #[test]
    fn validate_image_returns_png_dimensions_without_resize() {
        let input = source_png(64, 32);
        let info = validate_image(&input).unwrap();
        assert_eq!(info.width, 64);
        assert_eq!(info.height, 32);
        assert_eq!(info.format, "png");
    }

    #[test]
    fn validate_image_rejects_pixel_budget() {
        // 10_000 x 5_000 exceeds MAX_PIXELS (40_000_000).
        assert!(validate_dimensions(10_000, 5_000).is_err());
        let input = source_png(64, 32);
        assert!(validate_image(&input).is_ok());
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
