use std::collections::{BTreeMap, HashMap, HashSet};
use std::fs;
use std::io::{self, BufRead, BufReader, BufWriter, Write};
use std::path::{Component, Path, PathBuf};
use std::sync::{Arc, LazyLock, Mutex};
use std::time::{SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result, bail};
use ignore::{DirEntry, WalkBuilder};
use md5::{Digest, Md5};
use serde::{Deserialize, Serialize};

const MAX_TARGETS: usize = 4096;
const IGNORE_FILE: &str = ".ndignore";
const SPECIAL_DIRECTORIES: &[&str] = &[
    "$RECYCLE.BIN",
    "#snapshot",
    "@Recycle",
    "@Recently-Snapshot",
    ".git",
    ".streams",
    "lost+found",
];
include!(concat!(env!("OUT_DIR"), "/media_extensions.rs"));
const PLAYLIST_EXTENSIONS: &[&str] = &["m3u", "m3u8", "nsp"];

static AUDIO_EXTENSION_SET: LazyLock<HashSet<&'static str>> =
    LazyLock::new(|| AUDIO_EXTENSIONS.iter().copied().collect());
static IMAGE_EXTENSION_SET: LazyLock<HashSet<&'static str>> =
    LazyLock::new(|| IMAGE_EXTENSIONS.iter().copied().collect());
static PLAYLIST_EXTENSION_SET: LazyLock<HashSet<&'static str>> =
    LazyLock::new(|| PLAYLIST_EXTENSIONS.iter().copied().collect());

#[derive(Debug, Deserialize)]
pub(crate) struct ScanRequest {
    pub(crate) root: PathBuf,
    #[serde(default, deserialize_with = "deserialize_targets")]
    pub(crate) targets: Vec<String>,
    pub(crate) follow_symlinks: bool,
    pub(crate) ignore_dot_folders: bool,
    /// DB folder path -> content hash. Matching folders emit a lightweight summary.
    #[serde(default)]
    pub(crate) known_hashes: HashMap<String, String>,
    /// Parallel walk threads (0 = single-threaded).
    #[serde(default)]
    pub(crate) walk_threads: usize,
}

fn deserialize_targets<'de, D>(deserializer: D) -> Result<Vec<String>, D::Error>
where
    D: serde::Deserializer<'de>,
{
    Option::<Vec<String>>::deserialize(deserializer).map(|targets| targets.unwrap_or_default())
}

#[derive(Debug, Default, Serialize)]
pub struct Folder {
    pub(crate) path: String,
    pub(crate) mod_time_ns: i64,
    pub(crate) images_updated_at_ns: i64,
    pub(crate) num_playlists: usize,
    pub(crate) num_subfolders: usize,
    pub(crate) audio_files: BTreeMap<String, FileEntry>,
    pub(crate) image_files: BTreeMap<String, FileEntry>,
    /// Content hash matching scanner/folder_entry.go hash(), so Go can skip
    /// recomputing MD5 on every folder during change detection / persist.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub(crate) hash: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FileEntry {
    #[serde(default)]
    pub(crate) name: String,
    pub(crate) size: u64,
    pub(crate) mod_time_ns: i64,
}

impl FileEntry {
    pub fn new(name: impl Into<String>, size: u64, mod_time_ns: i64) -> Self {
        Self {
            name: name.into(),
            size,
            mod_time_ns,
        }
    }
}

#[derive(Debug, Serialize)]
pub(crate) struct FolderSummary {
    pub(crate) path: String,
    pub(crate) hash: String,
}

#[derive(Debug, Serialize)]
#[serde(tag = "kind", rename_all = "snake_case")]
pub(crate) enum Event<'a> {
    Folder { folder: &'a Folder },
    FolderSummary { folder: FolderSummary },
    Warning { message: &'a str },
    Error { message: &'a str },
    Done { folders: usize, files: usize },
}

pub(crate) trait EventSink {
    fn emit(&mut self, event: &Event<'_>) -> Result<()>;
}

struct JsonSink<'a, W: Write> {
    output: &'a mut W,
}

impl<W: Write> EventSink for JsonSink<'_, W> {
    fn emit(&mut self, event: &Event<'_>) -> Result<()> {
        write_event(self.output, event, true)
    }
}

pub fn run() -> Result<()> {
    let stdin = io::stdin();
    let stdout = io::stdout();
    let mut input = BufReader::with_capacity(64 * 1024, stdin.lock());
    let mut output = BufWriter::with_capacity(256 * 1024, stdout.lock());
    let mut line = String::with_capacity(4096);

    loop {
        line.clear();
        let read = input.read_line(&mut line).context("reading scan request")?;
        if read == 0 {
            return Ok(());
        }
        if line.trim().is_empty() {
            continue;
        }
        let request = match serde_json::from_str::<ScanRequest>(&line) {
            Ok(request) => request,
            Err(error) => {
                write_event(
                    &mut output,
                    &Event::Error {
                        message: &format!("decoding scan request: {error}"),
                    },
                    true,
                )?;
                continue;
            }
        };
        if let Err(error) = validate_request(&request) {
            write_event(
                &mut output,
                &Event::Error {
                    message: &format!("{error:#}"),
                },
                true,
            )?;
            continue;
        }
        if let Err(error) = run_scan(request, &mut output) {
            write_event(
                &mut output,
                &Event::Error {
                    message: &format!("{error:#}"),
                },
                true,
            )?;
        }
    }
}

pub(crate) fn validate_request(request: &ScanRequest) -> Result<()> {
    if request.targets.len() > MAX_TARGETS {
        bail!(
            "scan request has {} targets; maximum is {MAX_TARGETS}",
            request.targets.len()
        );
    }
    if !request.root.is_absolute() {
        bail!("music root must be absolute");
    }
    for target in &request.targets {
        let path = Path::new(target);
        if path.is_absolute()
            || path.components().any(|component| {
                matches!(
                    component,
                    Component::ParentDir | Component::RootDir | Component::Prefix(_)
                )
            })
        {
            bail!("scan target escapes music root: {target:?}");
        }
    }
    Ok(())
}

pub(crate) fn run_scan(request: ScanRequest, output: &mut impl Write) -> Result<()> {
    let mut sink = JsonSink { output };
    run_scan_into(request, &mut sink)
}

pub(crate) fn run_scan_into(request: ScanRequest, sink: &mut impl EventSink) -> Result<()> {
    let known_hashes = request.known_hashes.clone();
    let root = request
        .root
        .canonicalize()
        .with_context(|| format!("resolving music root {}", request.root.display()))?;
    let targets = if request.targets.is_empty() {
        vec![".".to_owned()]
    } else {
        request.targets.clone()
    };
    let ignore_cache = Arc::new(Mutex::new(HashMap::<PathBuf, bool>::new()));
    let mut folder_count = 0usize;
    let mut file_count = 0usize;

    for target in targets {
        let target_path = root.join(Path::new(&target));
        if !target_path.exists() {
            sink.emit(&Event::Warning {
                message: &format!("scan target does not exist: {target}"),
            })?;
            continue;
        }
        if request.walk_threads > 1 {
            let (folders, warnings) = collect_target(&root, &target, &request, &ignore_cache)?;
            for warning in &warnings {
                sink.emit(&Event::Warning { message: warning })?;
            }
            emit_collected_folders(
                &folders,
                &known_hashes,
                sink,
                &mut folder_count,
                &mut file_count,
            )?;
        } else {
            walk_target_post_order(
                &root,
                &target,
                &request,
                &known_hashes,
                &ignore_cache,
                sink,
                &mut folder_count,
                &mut file_count,
            )?;
        }
    }

    sink.emit(&Event::Done {
        folders: folder_count,
        files: file_count,
    })?;
    Ok(())
}

fn emit_collected_folders(
    folders: &BTreeMap<String, Folder>,
    known_hashes: &HashMap<String, String>,
    sink: &mut impl EventSink,
    folder_count: &mut usize,
    file_count: &mut usize,
) -> Result<()> {
    let mut ordered: Vec<&Folder> = folders.values().collect();
    ordered.sort_by(|left, right| {
        path_depth(&right.path)
            .cmp(&path_depth(&left.path))
            .then_with(|| left.path.cmp(&right.path))
    });
    for folder in ordered {
        emit_folder(sink, folder, known_hashes)?;
        *folder_count += 1;
        *file_count += folder_file_count(folder);
    }
    Ok(())
}

fn folder_file_count(folder: &Folder) -> usize {
    folder.audio_files.len() + folder.image_files.len() + folder.num_playlists
}

fn emit_folder(
    sink: &mut impl EventSink,
    folder: &Folder,
    known_hashes: &HashMap<String, String>,
) -> Result<()> {
    if known_hashes
        .get(&folder.path)
        .is_some_and(|known| known == &folder.hash)
    {
        sink.emit(&Event::FolderSummary {
            folder: FolderSummary {
                path: folder.path.clone(),
                hash: folder.hash.clone(),
            },
        })
    } else {
        sink.emit(&Event::Folder { folder })
    }
}

fn walk_target_post_order(
    root: &Path,
    target: &str,
    request: &ScanRequest,
    known_hashes: &HashMap<String, String>,
    ignore_cache: &Arc<Mutex<HashMap<PathBuf, bool>>>,
    sink: &mut impl EventSink,
    folder_count: &mut usize,
    file_count: &mut usize,
) -> Result<()> {
    let relative = normalize_relative(Path::new(target));
    let abs_path = root.join(Path::new(target));
    walk_folder_post_order(
        root,
        &abs_path,
        &relative,
        request,
        known_hashes,
        ignore_cache,
        sink,
        folder_count,
        file_count,
    )
}

fn walk_folder_post_order(
    root: &Path,
    abs_path: &Path,
    relative: &str,
    request: &ScanRequest,
    known_hashes: &HashMap<String, String>,
    ignore_cache: &Arc<Mutex<HashMap<PathBuf, bool>>>,
    sink: &mut impl EventSink,
    folder_count: &mut usize,
    file_count: &mut usize,
) -> Result<()> {
    let metadata =
        fs::metadata(abs_path).with_context(|| format!("reading {}", abs_path.display()))?;
    let mut folder = Folder {
        path: relative.to_owned(),
        mod_time_ns: system_time_ns(metadata.modified().unwrap_or(UNIX_EPOCH)),
        ..Folder::default()
    };
    let mut child_dirs = Vec::<String>::new();

    let mut builder = WalkBuilder::new(abs_path);
    builder
        .standard_filters(false)
        .add_custom_ignore_filename(IGNORE_FILE)
        .follow_links(request.follow_symlinks)
        .max_depth(Some(1))
        .sort_by_file_name(|left, right| left.cmp(right));
    let ignore_dot_folders = request.ignore_dot_folders;
    let follow_symlinks = request.follow_symlinks;
    let cache_for_filter = Arc::clone(ignore_cache);
    builder.filter_entry(move |entry| {
        allow_entry(
            entry,
            ignore_dot_folders,
            follow_symlinks,
            &cache_for_filter,
        )
    });

    for result in builder.build() {
        let entry = match result {
            Ok(entry) => entry,
            Err(error) => {
                sink.emit(&Event::Warning {
                    message: &format!("filesystem traversal warning: {error}"),
                })?;
                continue;
            }
        };
        if entry.depth() == 0 {
            continue;
        }
        let file_type = match entry.file_type() {
            Some(file_type) => file_type,
            None => continue,
        };
        let entry_metadata = match entry.metadata() {
            Ok(metadata) => metadata,
            Err(error) => {
                sink.emit(&Event::Warning {
                    message: &format!("{}: {error:#}", entry.path().display()),
                })?;
                continue;
            }
        };
        let mod_time_ns = system_time_ns(entry_metadata.modified().unwrap_or(UNIX_EPOCH));
        if file_type.is_dir() {
            let child_name = entry.file_name().to_string_lossy();
            let child_relative = join_relative(relative, &child_name);
            child_dirs.push(child_relative);
            continue;
        }

        let source_name = entry.file_name().to_string_lossy().into_owned();
        if source_name == IGNORE_FILE {
            continue;
        }
        let classification_name = if entry.path_is_symlink() {
            entry
                .path()
                .canonicalize()
                .ok()
                .and_then(|path| path.file_name().map(|name| name.to_owned()))
                .unwrap_or_else(|| entry.file_name().to_owned())
        } else {
            entry.file_name().to_owned()
        };
        let extension = Path::new(&classification_name)
            .extension()
            .and_then(|extension| extension.to_str())
            .map(str::to_ascii_lowercase)
            .unwrap_or_default();
        let file = FileEntry {
            name: source_name.clone(),
            size: entry_metadata.len(),
            mod_time_ns,
        };
        folder.mod_time_ns = folder.mod_time_ns.max(mod_time_ns);
        match extension.as_str() {
            extension if AUDIO_EXTENSION_SET.contains(extension) => {
                folder.audio_files.insert(source_name, file);
            }
            extension if IMAGE_EXTENSION_SET.contains(extension) => {
                folder.images_updated_at_ns = folder.images_updated_at_ns.max(mod_time_ns);
                folder.image_files.insert(source_name, file);
            }
            extension if PLAYLIST_EXTENSION_SET.contains(extension) => {
                folder.num_playlists += 1;
            }
            _ => {}
        }
    }

    for child_relative in &child_dirs {
        walk_folder_post_order(
            root,
            &root.join(Path::new(child_relative)),
            child_relative,
            request,
            known_hashes,
            ignore_cache,
            sink,
            folder_count,
            file_count,
        )?;
        folder.num_subfolders += 1;
    }

    folder.hash = folder_content_hash(&folder);
    emit_folder(sink, &folder, known_hashes)?;
    *folder_count += 1;
    *file_count += folder_file_count(&folder);
    Ok(())
}

fn join_relative(parent: &str, child: &str) -> String {
    if parent == "." {
        child.to_owned()
    } else {
        format!("{parent}/{child}")
    }
}

fn collect_target(
    root: &Path,
    target: &str,
    request: &ScanRequest,
    ignore_cache: &Arc<Mutex<HashMap<PathBuf, bool>>>,
) -> Result<(BTreeMap<String, Folder>, Vec<String>)> {
    let target_path = root.join(Path::new(target));
    let mut folders = BTreeMap::<String, Folder>::new();
    let mut warnings = Vec::<String>::new();

    let mut builder = WalkBuilder::new(&target_path);
    builder
        .standard_filters(false)
        .add_custom_ignore_filename(IGNORE_FILE)
        .follow_links(request.follow_symlinks)
        .sort_by_file_name(|left, right| left.cmp(right));
    if request.walk_threads > 1 {
        builder.threads(request.walk_threads);
    }
    let ignore_dot_folders = request.ignore_dot_folders;
    let follow_symlinks = request.follow_symlinks;
    let cache_for_filter = Arc::clone(ignore_cache);
    builder.filter_entry(move |entry| {
        allow_entry(
            entry,
            ignore_dot_folders,
            follow_symlinks,
            &cache_for_filter,
        )
    });

    for result in builder.build() {
        let entry = match result {
            Ok(entry) => entry,
            Err(error) => {
                warnings.push(format!("filesystem traversal warning: {error}"));
                continue;
            }
        };
        if let Err(error) = collect_entry(root, &entry, &mut folders, ignore_cache) {
            warnings.push(format!("{}: {error:#}", entry.path().display()));
        }
    }

    let paths: Vec<String> = folders.keys().cloned().collect();
    let folder_paths: HashSet<&str> = paths.iter().map(String::as_str).collect();
    for path in &paths {
        if path == "." {
            continue;
        }
        let parent = parent_path(path);
        if folder_paths.contains(parent.as_str())
            && let Some(folder) = folders.get_mut(&parent)
        {
            folder.num_subfolders += 1;
        }
    }

    for folder in folders.values_mut() {
        folder.hash = folder_content_hash(folder);
    }

    Ok((folders, warnings))
}

fn collect_scan(request: ScanRequest) -> Result<(BTreeMap<String, Folder>, Vec<String>)> {
    let root = request
        .root
        .canonicalize()
        .with_context(|| format!("resolving music root {}", request.root.display()))?;
    let targets = if request.targets.is_empty() {
        vec![".".to_owned()]
    } else {
        request.targets.clone()
    };
    let mut folders = BTreeMap::<String, Folder>::new();
    let mut warnings = Vec::<String>::new();
    let ignore_cache = Arc::new(Mutex::new(HashMap::<PathBuf, bool>::new()));

    for target in targets {
        let target_path = root.join(Path::new(&target));
        if !target_path.exists() {
            warnings.push(format!("scan target does not exist: {target}"));
            continue;
        }
        let (mut target_folders, mut target_warnings) =
            collect_target(&root, &target, &request, &ignore_cache)?;
        warnings.append(&mut target_warnings);
        folders.append(&mut target_folders);
    }

    Ok((folders, warnings))
}

/// Byte-for-byte match of scanner/folder_entry.go `hash()` after unixNanoTime
/// conversion (ns==0 → Go zero time year 0001).
pub fn folder_content_hash(folder: &Folder) -> String {
    let mut hasher = Md5::new();
    hash_folder_header(&mut hasher, folder);
    // BTreeMap iteration is sorted by key, matching Go's slices.Sort(mapKeys).
    for (name, file) in &folder.audio_files {
        hash_file_entry(&mut hasher, name, file);
    }
    for (name, file) in &folder.image_files {
        hash_file_entry(&mut hasher, name, file);
    }
    hex::encode(hasher.finalize())
}

#[derive(Debug, Deserialize)]
pub struct FolderHashInput {
    #[serde(default)]
    pub path: String,
    pub mod_time_ns: i64,
    #[serde(default)]
    pub images_updated_at_ns: i64,
    #[serde(default)]
    pub num_playlists: usize,
    #[serde(default)]
    pub num_subfolders: usize,
    #[serde(default)]
    pub audio_files: BTreeMap<String, FileEntry>,
    #[serde(default)]
    pub image_files: BTreeMap<String, FileEntry>,
}

pub fn folder_hash_from_input(input: &FolderHashInput) -> String {
    let folder = Folder {
        path: input.path.clone(),
        mod_time_ns: input.mod_time_ns,
        images_updated_at_ns: input.images_updated_at_ns,
        num_playlists: input.num_playlists,
        num_subfolders: input.num_subfolders,
        audio_files: input.audio_files.clone(),
        image_files: input.image_files.clone(),
        hash: String::new(),
    };
    folder_content_hash(&folder)
}

fn hash_folder_header(hasher: &mut Md5, folder: &Folder) {
    append_go_utc_time(hasher, folder.mod_time_ns);
    hasher.update(b":");
    write_decimal(hasher, folder.num_playlists as u64);
    hasher.update(b":");
    write_decimal(hasher, folder.num_subfolders as u64);
    hasher.update(b":");
    append_go_utc_time(hasher, folder.images_updated_at_ns);
}

fn hash_file_entry(hasher: &mut Md5, name: &str, file: &FileEntry) {
    hasher.update(name.as_bytes());
    hasher.update(b":");
    write_decimal(hasher, file.size);
    hasher.update(b":");
    append_go_utc_time(hasher, file.mod_time_ns);
}

fn write_decimal(hasher: &mut Md5, value: u64) {
    let mut buffer = [0u8; 20];
    let mut index = buffer.len();
    let mut current = value;
    loop {
        index -= 1;
        buffer[index] = b'0' + (current % 10) as u8;
        current /= 10;
        if current == 0 {
            break;
        }
    }
    hasher.update(&buffer[index..]);
}

/// Formats like Go's `time.Time.UTC().String()` / `fmt %s` with layout
/// `2006-01-02 15:04:05.999999999 -0700 MST` (trailing fractional zeros stripped).
fn go_utc_time_string(unix_ns: i64) -> String {
    let mut buffer = Vec::with_capacity(40);
    write_go_utc_time_bytes(|slice| buffer.extend_from_slice(slice), unix_ns);
    // SAFETY: only ASCII digits, punctuation, and spaces are written.
    unsafe { String::from_utf8_unchecked(buffer) }
}

fn append_go_utc_time(hasher: &mut Md5, unix_ns: i64) {
    write_go_utc_time_bytes(|slice| hasher.update(slice), unix_ns);
}

fn write_go_utc_time_bytes(mut write: impl FnMut(&[u8]), unix_ns: i64) {
    if unix_ns == 0 {
        write(b"0001-01-01 00:00:00 +0000 UTC");
        return;
    }
    let secs = unix_ns.div_euclid(1_000_000_000);
    let nsec = unix_ns.rem_euclid(1_000_000_000) as u32;
    let days = secs.div_euclid(86_400);
    let day_secs = secs.rem_euclid(86_400) as u32;
    let (year, month, day) = civil_from_days(days);
    let hour = day_secs / 3600;
    let minute = (day_secs % 3600) / 60;
    let second = day_secs % 60;
    write_date_time(&mut write, year, month, day, hour, minute, second);
    if nsec != 0 {
        write(b".");
        write_fractional_nanos(&mut write, nsec);
    }
    write(b" +0000 UTC");
}

fn write_date_time(
    write: &mut impl FnMut(&[u8]),
    year: i32,
    month: u32,
    day: u32,
    hour: u32,
    minute: u32,
    second: u32,
) {
    write_padded(write, year as u64, 4);
    write(b"-");
    write_padded(write, month as u64, 2);
    write(b"-");
    write_padded(write, day as u64, 2);
    write(b" ");
    write_padded(write, hour as u64, 2);
    write(b":");
    write_padded(write, minute as u64, 2);
    write(b":");
    write_padded(write, second as u64, 2);
}

fn write_padded(write: &mut impl FnMut(&[u8]), value: u64, width: usize) {
    let mut buffer = [0u8; 20];
    let mut index = buffer.len();
    let mut current = value;
    if current == 0 {
        index -= 1;
        buffer[index] = b'0';
    } else {
        while current > 0 {
            index -= 1;
            buffer[index] = b'0' + (current % 10) as u8;
            current /= 10;
        }
    }
    let digits = &buffer[index..];
    for _ in digits.len()..width {
        write(b"0");
    }
    write(digits);
}

fn write_fractional_nanos(write: &mut impl FnMut(&[u8]), nsec: u32) {
    let mut buffer = [0u8; 9];
    let mut current = nsec;
    for index in (0..9).rev() {
        buffer[index] = b'0' + (current % 10) as u8;
        current /= 10;
    }
    let trimmed = buffer
        .iter()
        .rposition(|byte| *byte != b'0')
        .map(|index| index + 1)
        .unwrap_or(0);
    write(&buffer[..trimmed]);
}

/// Howard Hinnant civil_from_days: `z` is days since 1970-01-01.
fn civil_from_days(z: i64) -> (i32, u32, u32) {
    let z = z + 719_468;
    let era = if z >= 0 { z } else { z - 146_096 } / 146_097;
    let doe = (z - era * 146_097) as u32;
    let yoe = (doe - doe / 1460 + doe / 36524 - doe / 146_096) / 365;
    let y = era * 400 + yoe as i64;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = doy - (153 * mp + 2) / 5 + 1;
    let m = if mp < 10 { mp + 3 } else { mp - 9 };
    let y = if m <= 2 { y + 1 } else { y };
    (y as i32, m, d)
}

fn allow_entry(
    entry: &DirEntry,
    ignore_dot_folders: bool,
    follow_symlinks: bool,
    ignore_cache: &Arc<Mutex<HashMap<PathBuf, bool>>>,
) -> bool {
    if entry.depth() == 0 {
        return !directory_has_empty_ignore(entry.path(), ignore_cache);
    }
    let Some(name) = entry.file_name().to_str() else {
        return false;
    };
    if entry.path_is_symlink() && !follow_symlinks {
        return false;
    }
    let is_dir = entry.file_type().is_some_and(|kind| kind.is_dir());
    if is_dir
        && SPECIAL_DIRECTORIES
            .iter()
            .any(|ignored| ignored.eq_ignore_ascii_case(name))
    {
        return false;
    }
    if is_dot_entry(name) && (!is_dir || ignore_dot_folders) {
        return false;
    }
    if is_dir && directory_has_empty_ignore(entry.path(), ignore_cache) {
        return false;
    }
    true
}

fn directory_has_empty_ignore(
    path: &Path,
    ignore_cache: &Arc<Mutex<HashMap<PathBuf, bool>>>,
) -> bool {
    if let Some(cached) = ignore_cache
        .lock()
        .ok()
        .and_then(|cache| cache.get(path).copied())
    {
        return cached;
    }
    let ignore_path = path.join(IGNORE_FILE);
    let ignored = match fs::read_to_string(ignore_path) {
        Ok(contents) => !contents
            .lines()
            .map(str::trim)
            .any(|line| !line.is_empty() && !line.starts_with('#')),
        Err(_) => false,
    };
    if let Ok(mut cache) = ignore_cache.lock() {
        cache.insert(path.to_path_buf(), ignored);
    }
    ignored
}

fn is_dot_entry(name: &str) -> bool {
    name != "." && name.starts_with('.') && !name.starts_with("..")
}

fn collect_entry(
    root: &Path,
    entry: &DirEntry,
    folders: &mut BTreeMap<String, Folder>,
    ignore_cache: &Arc<Mutex<HashMap<PathBuf, bool>>>,
) -> Result<()> {
    let relative = entry
        .path()
        .strip_prefix(root)
        .context("entry is outside music root")?;
    let relative_path = normalize_relative(relative);
    let file_type = entry.file_type().context("entry has no file type")?;
    let metadata = entry.metadata().context("reading metadata")?;
    let mod_time_ns = system_time_ns(metadata.modified().unwrap_or(UNIX_EPOCH));

    if file_type.is_dir() {
        let folder = folders.entry(relative_path.clone()).or_default();
        folder.path = relative_path;
        folder.mod_time_ns = folder.mod_time_ns.max(mod_time_ns);
        let _ = ignore_cache;
        return Ok(());
    }

    let parent = normalize_relative(relative.parent().unwrap_or(Path::new(".")));
    let Some(folder) = folders.get_mut(&parent) else {
        return Ok(());
    };
    let source_name = entry.file_name().to_string_lossy().into_owned();
    if source_name == IGNORE_FILE {
        return Ok(());
    }
    let classification_name = if entry.path_is_symlink() {
        entry
            .path()
            .canonicalize()
            .ok()
            .and_then(|path| path.file_name().map(|name| name.to_owned()))
            .unwrap_or_else(|| entry.file_name().to_owned())
    } else {
        entry.file_name().to_owned()
    };
    let extension = Path::new(&classification_name)
        .extension()
        .and_then(|extension| extension.to_str())
        .map(str::to_ascii_lowercase)
        .unwrap_or_default();
    let file = FileEntry {
        name: source_name.clone(),
        size: metadata.len(),
        mod_time_ns,
    };
    folder.mod_time_ns = folder.mod_time_ns.max(mod_time_ns);
    match extension.as_str() {
        extension if AUDIO_EXTENSION_SET.contains(extension) => {
            folder.audio_files.insert(source_name, file);
        }
        extension if IMAGE_EXTENSION_SET.contains(extension) => {
            folder.images_updated_at_ns = folder.images_updated_at_ns.max(mod_time_ns);
            folder.image_files.insert(source_name, file);
        }
        extension if PLAYLIST_EXTENSION_SET.contains(extension) => {
            folder.num_playlists += 1;
        }
        _ => {}
    }
    Ok(())
}

fn normalize_relative(path: &Path) -> String {
    let value = path.to_string_lossy().replace('\\', "/");
    if value.is_empty() {
        ".".to_owned()
    } else {
        value
    }
}

fn parent_path(path: &str) -> String {
    Path::new(path)
        .parent()
        .map(normalize_relative)
        .unwrap_or_else(|| ".".to_owned())
}

fn path_depth(path: &str) -> usize {
    if path == "." {
        0
    } else {
        Path::new(path).components().count()
    }
}

fn system_time_ns(value: SystemTime) -> i64 {
    match value.duration_since(UNIX_EPOCH) {
        Ok(duration) => duration.as_nanos().min(i64::MAX as u128) as i64,
        Err(error) => -(error.duration().as_nanos().min(i64::MAX as u128) as i64),
    }
}

fn write_event(output: &mut impl Write, event: &Event<'_>, flush: bool) -> Result<()> {
    serde_json::to_writer(&mut *output, event)?;
    output.write_all(b"\n")?;
    if flush {
        output.flush()?;
    }
    Ok(())
}

pub fn bench_folder(file_count: usize) -> Folder {
    let mut folder = Folder {
        path: "Artist/Album".to_owned(),
        mod_time_ns: 1_710_505_845_123_456_789,
        images_updated_at_ns: 1_710_505_845_000_000_000,
        num_playlists: 1,
        num_subfolders: 2,
        ..Folder::default()
    };
    for index in 0..file_count {
        let name = format!("track-{index:04}.flac");
        folder.audio_files.insert(
            name.clone(),
            FileEntry {
                name,
                size: 4_000_000 + index as u64,
                mod_time_ns: 1_710_505_845_000_000_000 + index as i64,
            },
        );
    }
    folder.image_files.insert(
        "cover.jpg".to_owned(),
        FileEntry {
            name: "cover.jpg".to_owned(),
            size: 50_000,
            mod_time_ns: 1_710_505_845_123_456_789,
        },
    );
    folder
}

#[cfg(test)]
mod tests {
    use super::*;

    fn temporary_music_root() -> PathBuf {
        std::env::temp_dir().join(format!(
            "navidrome-rust-scanner-{}-{}",
            std::process::id(),
            SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ))
    }

    #[test]
    fn accepts_null_targets() {
        let request = serde_json::from_str::<ScanRequest>(
            r#"{"root":"/music","targets":null,"follow_symlinks":false,"ignore_dot_folders":true}"#,
        )
        .expect("null targets should deserialize as empty");
        assert!(request.targets.is_empty());
        assert!(validate_request(&request).is_ok());
    }

    #[test]
    fn validates_relative_targets() {
        let root = if cfg!(windows) {
            PathBuf::from(r"C:\music")
        } else {
            PathBuf::from("/music")
        };
        assert!(
            validate_request(&ScanRequest {
                root: root.clone(),
                targets: vec!["Artist/Album".to_owned()],
                follow_symlinks: false,
                ignore_dot_folders: true,
                known_hashes: HashMap::new(),
                walk_threads: 0,
            })
            .is_ok()
        );
        assert!(
            validate_request(&ScanRequest {
                root,
                targets: vec!["../outside".to_owned()],
                follow_symlinks: false,
                ignore_dot_folders: true,
                known_hashes: HashMap::new(),
                walk_threads: 0,
            })
            .is_err()
        );
    }

    #[test]
    fn dot_policy_matches_scanner_semantics() {
        assert!(is_dot_entry(".hidden"));
        assert!(!is_dot_entry("..Album"));
        assert!(!is_dot_entry("regular"));
        let cache = HashMap::new();
        let cache = Arc::new(Mutex::new(cache));
        let root = temporary_music_root();
        fs::create_dir_all(root.join("cached")).unwrap();
        fs::write(root.join("cached/.ndignore"), b"\n").unwrap();
        assert!(directory_has_empty_ignore(
            root.join("cached").as_path(),
            &cache
        ));
        assert!(cache.lock().unwrap().contains_key(&root.join("cached")));
        fs::remove_dir_all(root).unwrap();
    }

    #[test]
    fn normalizes_root_and_nested_paths() {
        assert_eq!(normalize_relative(Path::new("")), ".");
        assert_eq!(
            normalize_relative(Path::new("Artist/Album")),
            "Artist/Album"
        );
        assert_eq!(parent_path("Artist/Album"), "Artist");
    }

    #[test]
    fn run_scan_emits_done_event() {
        let root = temporary_music_root();
        fs::create_dir_all(root.join("Album")).unwrap();
        fs::write(root.join("Album/track.mp3"), b"audio").unwrap();

        let mut output = Vec::new();
        run_scan(
            ScanRequest {
                root: root.clone(),
                targets: vec![".".to_owned()],
                follow_symlinks: false,
                ignore_dot_folders: true,
                known_hashes: HashMap::new(),
                walk_threads: 0,
            },
            &mut output,
        )
        .unwrap();

        let text = String::from_utf8(output).expect("utf8 scan output");
        assert!(text.contains("\"kind\":\"done\""));
        fs::remove_dir_all(root).unwrap();
    }

    #[test]
    fn post_order_emits_deepest_folders_first() {
        let root = temporary_music_root();
        fs::create_dir_all(root.join("Artist/Album")).unwrap();
        fs::write(root.join("Artist/Album/track.mp3"), b"audio").unwrap();

        let mut output = Vec::new();
        run_scan(
            ScanRequest {
                root: root.clone(),
                targets: vec![".".to_owned()],
                follow_symlinks: false,
                ignore_dot_folders: true,
                known_hashes: HashMap::new(),
                walk_threads: 0,
            },
            &mut output,
        )
        .unwrap();

        let mut folder_paths = Vec::new();
        for line in String::from_utf8(output).expect("utf8 scan output").lines() {
            let value: serde_json::Value = serde_json::from_str(line).expect("json line");
            if value.get("kind").and_then(|kind| kind.as_str()) == Some("folder") {
                folder_paths.push(
                    value["folder"]["path"]
                        .as_str()
                        .expect("folder path")
                        .to_owned(),
                );
            }
        }
        assert_eq!(
            folder_paths,
            vec![
                "Artist/Album".to_owned(),
                "Artist".to_owned(),
                ".".to_owned(),
            ]
        );
        fs::remove_dir_all(root).unwrap();
    }

    #[test]
    fn traverses_classifies_and_applies_ndignore() {
        let root = temporary_music_root();
        fs::create_dir_all(root.join("Artist/Album/ignored")).unwrap();
        fs::create_dir_all(root.join("Artist/.Hidden Album")).unwrap();
        fs::create_dir_all(root.join("Linked Source")).unwrap();
        fs::write(root.join("Artist/Album/.ndignore"), b"ignored/\n").unwrap();
        fs::write(root.join("Artist/Album/song.flac"), b"audio").unwrap();
        fs::write(root.join("Artist/Album/lossless.alac"), b"audio").unwrap();
        fs::write(root.join("Artist/Album/cover.jpg"), b"image").unwrap();
        fs::write(root.join("Artist/Album/cover.jxl"), b"image").unwrap();
        fs::write(root.join("Artist/Album/list.m3u8"), b"playlist").unwrap();
        fs::write(root.join("Artist/Album/not-supported.pls"), b"playlist").unwrap();
        fs::write(root.join("Artist/Album/ignored/skip.mp3"), b"ignored").unwrap();
        fs::write(root.join("Artist/.Hidden Album/hidden.mp3"), b"hidden").unwrap();
        fs::write(root.join("Linked Source/linked.opus"), b"linked").unwrap();
        #[cfg(unix)]
        {
            std::os::unix::fs::symlink(
                root.join("Artist/Album/song.flac"),
                root.join("Artist/Album/linked.flac"),
            )
            .unwrap();
            std::os::unix::fs::symlink(
                root.join("Linked Source"),
                root.join("Artist/Linked Album"),
            )
            .unwrap();
        }

        let (folders, warnings) = collect_scan(ScanRequest {
            root: root.clone(),
            targets: vec![".".to_owned()],
            follow_symlinks: false,
            ignore_dot_folders: true,
            known_hashes: HashMap::new(),
            walk_threads: 0,
        })
        .unwrap();

        assert!(warnings.is_empty(), "{warnings:?}");
        let album = folders.get("Artist/Album").unwrap();
        assert!(album.audio_files.contains_key("song.flac"));
        assert!(album.audio_files.contains_key("lossless.alac"));
        #[cfg(unix)]
        assert!(!album.audio_files.contains_key("linked.flac"));
        #[cfg(unix)]
        assert!(!folders.contains_key("Artist/Linked Album"));
        assert!(album.image_files.contains_key("cover.jpg"));
        assert!(album.image_files.contains_key("cover.jxl"));
        assert_eq!(album.num_playlists, 1);
        assert!(!folders.contains_key("Artist/Album/ignored"));
        assert!(!folders.contains_key("Artist/.Hidden Album"));

        #[cfg(unix)]
        {
            let (linked_folders, linked_warnings) = collect_scan(ScanRequest {
                root: root.clone(),
                targets: vec!["Artist".to_owned()],
                follow_symlinks: true,
                ignore_dot_folders: true,
                known_hashes: HashMap::new(),
                walk_threads: 0,
            })
            .unwrap();
            assert!(linked_warnings.is_empty(), "{linked_warnings:?}");
            assert!(
                linked_folders["Artist/Album"]
                    .audio_files
                    .contains_key("linked.flac")
            );
            assert!(
                linked_folders["Artist/Linked Album"]
                    .audio_files
                    .contains_key("linked.opus")
            );
        }

        fs::remove_dir_all(root).unwrap();
    }

    #[test]
    fn folder_content_hash_matches_go_reference() {
        assert_eq!(go_utc_time_string(0), "0001-01-01 00:00:00 +0000 UTC");
        assert_eq!(
            go_utc_time_string(1_710_505_845_000_000_000),
            "2024-03-15 12:30:45 +0000 UTC"
        );
        assert_eq!(
            go_utc_time_string(1_710_505_845_123_456_789),
            "2024-03-15 12:30:45.123456789 +0000 UTC"
        );

        let mut folder = Folder {
            mod_time_ns: 1_710_505_845_123_456_789,
            images_updated_at_ns: 0,
            num_playlists: 1,
            num_subfolders: 2,
            ..Folder::default()
        };
        folder.audio_files.insert(
            "a.mp3".to_owned(),
            FileEntry {
                name: "a.mp3".to_owned(),
                size: 100,
                mod_time_ns: 1_710_505_845_000_000_000,
            },
        );
        folder.audio_files.insert(
            "b.flac".to_owned(),
            FileEntry {
                name: "b.flac".to_owned(),
                size: 200,
                mod_time_ns: 1_710_505_845_123_456_789,
            },
        );
        folder.image_files.insert(
            "cover.jpg".to_owned(),
            FileEntry {
                name: "cover.jpg".to_owned(),
                size: 50,
                mod_time_ns: 1_710_505_845_123_456_789,
            },
        );
        assert_eq!(
            folder_content_hash(&folder),
            "df67bbdb7506a6796a849caf4452a4a7"
        );
        assert_eq!(
            folder_content_hash(&Folder::default()),
            "c9e122666846c6107bcede3a959aa808"
        );
    }
}
