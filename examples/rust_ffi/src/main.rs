//! RustyGo Polyglot FFI Demonstration
//! Shows calling RustyGo's arena engine directly from idiomatic Rust.

use std::ffi::c_void;

#[repr(C)]
struct RustyArena {
    handle: usize,
}

extern "C" {
    fn rustygo_arena_new(size: i64) -> usize;
    fn rustygo_arena_alloc(handle: usize, size: i64, align: i32) -> *mut c_void;
    fn rustygo_arena_reset(handle: usize);
    fn rustygo_arena_close(handle: usize);
    fn rustygo_arena_used_bytes(handle: usize) -> i64;
}

impl RustyArena {
    pub fn new(size: i64) -> Self {
        let handle = unsafe { rustygo_arena_new(size) };
        Self { handle }
    }

    pub fn alloc(&self, size: i64, align: i32) -> *mut u8 {
        unsafe { rustygo_arena_alloc(self.handle, size, align) as *mut u8 }
    }

    pub fn reset(&self) {
        unsafe { rustygo_arena_reset(self.handle) };
    }

    pub fn used_bytes(&self) -> i64 {
        unsafe { rustygo_arena_used_bytes(self.handle) }
    }
}

impl Drop for RustyArena {
    fn drop(&mut self) {
        unsafe { rustygo_arena_close(self.handle) };
    }
}

fn main() {
    println!("🦀 Initializing 64KB RustyGo Arena from Rust...");
    let arena = RustyArena::new(64 * 1024);

    // Allocate 128 bytes with 64-byte SIMD/cacheline alignment
    let ptr = arena.alloc(128, 64);
    assert!(!ptr.is_null());
    assert_eq!(ptr as usize % 64, 0, "Memory must be 64-byte aligned");

    unsafe {
        // Write message to arena memory from Rust
        let msg = b"Zero-copy from Rust to RustyGo!";
        std::ptr::copy_nonoverlapping(msg.as_ptr(), ptr, msg.len());

        let slice = std::slice::from_raw_parts(ptr, msg.len());
        println!("🚀 Read back from Arena: {}", std::str::from_utf8(slice).unwrap());
    }

    println!("📊 Used bytes: {}", arena.used_bytes());
    arena.reset();
    println!("🔄 Arena reset. New used bytes: {}", arena.used_bytes());
}
