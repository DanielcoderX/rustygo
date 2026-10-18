use std::env;
use std::fs;
use std::path::PathBuf;

fn main() {
    let manifest_dir = env::var("CARGO_MANIFEST_DIR").unwrap();
    println!("cargo:rustc-link-search=native={}", manifest_dir);
    println!("cargo:rustc-link-lib=dylib=rustygo");

    // Copy DLL to target/debug or target/release so binary finds it when run
    if let Ok(out_dir) = env::var("OUT_DIR") {
        let mut target_dir = PathBuf::from(out_dir);
        // OUT_DIR is target/debug/build/<pkg>/out -> traverse up 3 levels
        target_dir.pop();
        target_dir.pop();
        target_dir.pop();

        let src_dll = PathBuf::from(&manifest_dir).join("rustygo.dll");
        let dst_dll = target_dir.join("rustygo.dll");
        if src_dll.exists() {
            let _ = fs::copy(src_dll, dst_dll);
        }
    }
}
