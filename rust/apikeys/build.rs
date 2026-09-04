fn main() -> Result<(), Box<dyn std::error::Error>> {
    let protoc = protoc_bin_vendored::protoc_bin_path()?;
    unsafe {
        std::env::set_var("PROTOC", protoc);
    }
    let proto = "../../proto/navidrome/apikeys/v1/apikeys.proto";
    println!("cargo:rerun-if-changed={proto}");
    tonic_prost_build::configure()
        .build_server(true)
        .build_client(false)
        .compile_protos(&[proto], &["../../proto"])?;
    Ok(())
}
