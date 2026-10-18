#ifndef RUSTYGO_H
#define RUSTYGO_H

#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

/**
 * @brief Creates a new RustyGo arena bump allocator.
 * 
 * @param size Initial chunk size in bytes (e.g. 65536 for 64KB).
 * @return uintptr_t Opaque handle to the arena instance.
 */
uintptr_t rustygo_arena_new(int64_t size);

/**
 * @brief Allocates aligned memory from the arena.
 * 
 * @param handle Opaque arena handle returned by rustygo_arena_new.
 * @param size Number of bytes to allocate.
 * @param align Power-of-two memory alignment (e.g. 8, 16, 64 for cacheline).
 * @return void* Pointer to the allocated memory buffer, or NULL on OOM.
 */
void* rustygo_arena_alloc(uintptr_t handle, int64_t size, int32_t align);

/**
 * @brief Resets all allocation offsets in the arena to 0 for instant reuse.
 * 
 * @param handle Opaque arena handle.
 */
void rustygo_arena_reset(uintptr_t handle);

/**
 * @brief Closes the arena and releases all backing memory to the operating system.
 * 
 * @param handle Opaque arena handle.
 */
void rustygo_arena_close(uintptr_t handle);

/**
 * @brief Returns the total number of bytes currently allocated in the arena.
 * 
 * @param handle Opaque arena handle.
 * @return int64_t Number of bytes allocated.
 */
int64_t rustygo_arena_used_bytes(uintptr_t handle);

#ifdef __cplusplus
}
#endif

#endif /* RUSTYGO_H */
