cmake_minimum_required(VERSION 3.16)
project(bse-hft-cpp VERSION 1.0.0 LANGUAGES CXX)

# C++17 required for std::filesystem, structured bindings, etc.
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# Build type
if(NOT CMAKE_BUILD_TYPE)
    set(CMAKE_BUILD_TYPE Release)
endif()

# Compiler flags
if(MSVC)
    # MSVC optimizations
    set(CMAKE_CXX_FLAGS_RELEASE "/O2 /Ob2 /Oi /Ot /GL /DNDEBUG /fp:fast /arch:AVX2")
    set(CMAKE_EXE_LINKER_FLAGS_RELEASE "/LTCG")
    add_definitions(-D_WIN32_WINNT=0x0A00)  # Windows 10
    add_definitions(-DNOMINMAX)              # Disable min/max macros
    add_definitions(-D_CRT_SECURE_NO_WARNINGS)
else()
    # GCC/Clang optimizations
    set(CMAKE_CXX_FLAGS_RELEASE "-O3 -march=native -mtune=native -flto -ffast-math -DNDEBUG")
    set(CMAKE_CXX_FLAGS_DEBUG "-g -O0 -fsanitize=address -fno-omit-frame-pointer")
endif()

# Include directories
include_directories(${CMAKE_SOURCE_DIR}/include)

# Source files (most code is header-only for inlining)
set(SOURCES
    src/main.cpp
)

# Main executable
add_executable(bse-hft-cpp ${SOURCES})

# Platform-specific libraries
if(WIN32)
    target_link_libraries(bse-hft-cpp ws2_32 wsock32)
else()
    target_link_libraries(bse-hft-cpp pthread)
endif()

# Copy config.json to build directory
configure_file(${CMAKE_SOURCE_DIR}/config.json ${CMAKE_BINARY_DIR}/config.json COPYONLY)

# Install
install(TARGETS bse-hft-cpp DESTINATION bin)

# Print configuration
message(STATUS "Build type: ${CMAKE_BUILD_TYPE}")
message(STATUS "C++ Compiler: ${CMAKE_CXX_COMPILER_ID} ${CMAKE_CXX_COMPILER_VERSION}")
message(STATUS "C++ Flags: ${CMAKE_CXX_FLAGS_${CMAKE_BUILD_TYPE}}")
