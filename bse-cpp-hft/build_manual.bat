@echo off
REM Build script for BSE HFT C++ - Manual Environment Setup
REM Uses Visual Studio 2026 Insiders MSVC compiler

echo ============================================
echo BSE HFT C++ Build Script
echo ============================================

REM Visual Studio 2026 Insiders paths
set "MSVC_VER=14.50.35717"
set "WINSDK_VER=10.0.26100.0"
set "VS_PATH=C:\Program Files\Microsoft Visual Studio\18\Insiders"
set "MSVC_PATH=%VS_PATH%\VC\Tools\MSVC\%MSVC_VER%"
set "WINSDK_PATH=C:\Program Files (x86)\Windows Kits\10"

REM Set PATH for compiler tools
set "PATH=%MSVC_PATH%\bin\Hostx64\x64;%PATH%"
set "PATH=%WINSDK_PATH%\bin\%WINSDK_VER%\x64;%PATH%"

REM Set INCLUDE paths
set "INCLUDE=%MSVC_PATH%\include"
set "INCLUDE=%INCLUDE%;%WINSDK_PATH%\Include\%WINSDK_VER%\ucrt"
set "INCLUDE=%INCLUDE%;%WINSDK_PATH%\Include\%WINSDK_VER%\um"
set "INCLUDE=%INCLUDE%;%WINSDK_PATH%\Include\%WINSDK_VER%\shared"

REM Set LIB paths
set "LIB=%MSVC_PATH%\lib\x64"
set "LIB=%LIB%;%WINSDK_PATH%\Lib\%WINSDK_VER%\ucrt\x64"
set "LIB=%LIB%;%WINSDK_PATH%\Lib\%WINSDK_VER%\um\x64"

echo.
echo [1/3] Verifying compiler...
cl 2>&1 | findstr "Version"
if errorlevel 1 (
    echo ERROR: cl.exe not available
    echo Make sure Visual Studio C++ tools are installed
    exit /b 1
)

echo.
echo [2/3] Compiling source files...
cd /d "%~dp0"

REM Create output directory
if not exist "bin" mkdir bin

REM Compile with full optimizations for HFT
cl /nologo /std:c++17 /O2 /Oi /Ot /GL /GF /EHsc /MT ^
   /D_WIN32_WINNT=0x0A00 /DNOMINMAX /DNDEBUG ^
   /I"%~dp0include" ^
   /Fe"bin\bse-hft-cpp.exe" ^
   src\main.cpp ^
   /link /LTCG /OPT:REF /OPT:ICF ws2_32.lib

if errorlevel 1 (
    echo.
    echo ERROR: Compilation failed
    exit /b 1
)

echo.
echo [3/3] Build complete!
echo ============================================
echo Executable: bin\bse-hft-cpp.exe
echo ============================================
echo.
dir bin\*.exe
