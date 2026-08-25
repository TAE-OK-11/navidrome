use std::io::{self, BufRead, BufReader, BufWriter, Cursor, Read, Write};

use anyhow::{Context, Result, bail};
use fast_image_resize as fir;
use image::codecs::jpeg::JpegEncoder;
use image::codecs::png::PngEncoder;
use image::{ExtendedColorType, ImageEncoder, ImageReader, Limits};
use serde::{Deserialize, Serialize};

const MAX_INPUT_BYTES: usize = 128 * 1024 * 1024;
const MAX_OUTPUT_BYTES: usize = 64 * 1024 * 1024;
const MAX_DIMENSION: u32 = 16_384;
const MAX_PIXELS: u64 = 40_000_000;

#[derive(Debug, Deserialize)]
struct ImageRequest {
    input_size: usize,
    size: u32,
    square: bool,
    quality: u8,
    format: OutputFormat,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "snake_case")]
enum OutputFormat {
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

        let mut encoded = vec![0; request.input_size];
        input
            .read_exact(&mut encoded)
            .context("reading framed image payload")?;

        match resize(&encoded, &request) {
            Ok(resized) => write_success(&mut output, &resized)?,
            Err(error) => write_error(&mut output, format!("{error:#}"))?,
        }
        output.flush().context("flushing image response")?;
    }
}

fn validate_request(request: &ImageRequest) -> Result<()> {
    if request.input_size == 0 || request.input_size > MAX_INPUT_BYTES {
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

fn resize(encoded: &[u8], request: &ImageRequest) -> Result<Vec<u8>> {
    let dimensions_reader = ImageReader::new(Cursor::new(encoded))
        .with_guessed_format()
        .context("detecting image format")?;
    let (src_width, src_height) = dimensions_reader
        .into_dimensions()
        .context("reading image dimensions")?;
    validate_dimensions(src_width, src_height)?;

    let original_size = src_width.max(src_height);
    let target_size = request.size.min(original_size);
    if target_size == original_size && !request.square {
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

    let (resized_width, resized_height) = fit_dimensions(src_width, src_height, target_size);
    let source = fir::images::Image::from_vec_u8(
        src_width,
        src_height,
        rgba.into_raw(),
        fir::PixelType::U8x4,
    )
    .context("creating resize source")?;
    let mut resized = fir::images::Image::new(resized_width, resized_height, fir::PixelType::U8x4);
    let options = fir::ResizeOptions::new()
        .resize_alg(fir::ResizeAlg::Convolution(fir::FilterType::CatmullRom));
    fir::Resizer::new()
        .resize(&source, &mut resized, &options)
        .context("resizing image")?;

    let (pixels, output_width, output_height) = if request.square {
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
                .context("encoding WebP")?;
            output.extend_from_slice(&encoded);
        }
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
    fn resizes_and_centers_on_square_canvas() {
        let input = source_png(80, 40);
        let output = resize(
            &input,
            &ImageRequest {
                input_size: input.len(),
                size: 20,
                square: true,
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
