# BSE Go HFT - Makefile
# Build and run commands for the HFT system

.PHONY: all build clean run server benchmark test help

# Default target
all: build

# Build all executables
build:
	go build -o benchmark.exe ./cmd/benchmark/
	go build -o hft-server.exe ./cmd/hft-server/

# Build benchmark only
build-benchmark:
	go build -o benchmark.exe ./cmd/benchmark/

# Build HFT server only
build-server:
	go build -o hft-server.exe ./cmd/hft-server/

# Build with optimizations
build-release:
	go build -ldflags="-s -w" -o benchmark.exe ./cmd/benchmark/
	go build -ldflags="-s -w" -o hft-server.exe ./cmd/hft-server/

# Clean build artifacts
clean:
	del /Q benchmark.exe 2>nul || true
	del /Q hft-server.exe 2>nul || true
	del /Q *.log 2>nul || true

# ================================================================================
# RUN WITH GO RUN (Like Python: python main.py)
# ================================================================================

# Run HFT server directly with F&O feed (default: port 26002)
run:
	go run ./cmd/hft-server/main.go -feed FO

# Run HFT server with F&O feed (port 26002)
run-fo:
	go run ./cmd/hft-server/main.go -feed FO

# Run HFT server with Equity feed (port 26001)
run-eq:
	go run ./cmd/hft-server/main.go -feed EQ

# Run HFT server for 5 minutes (F&O)
run-5m:
	go run ./cmd/hft-server/main.go -feed FO -duration 5m

# Run HFT server for 5 minutes (Equity)
run-eq-5m:
	go run ./cmd/hft-server/main.go -feed EQ -duration 5m

# Run benchmark directly
run-benchmark:
	go run ./cmd/benchmark/main.go

# Run benchmark for 60 seconds
run-benchmark-60s:
	go run ./cmd/benchmark/main.go -duration 60s

# ================================================================================
# HFT SERVER - Complete Pipeline (Download CM + Connect + Decode + Save CSV)
# ================================================================================

# Run HFT server with F&O feed (port 26002)
server:
	.\hft-server.exe -feed FO

# Run HFT server with Equity feed (port 26001)
server-eq:
	.\hft-server.exe -feed EQ

# Run HFT server for 5 minutes (F&O)
server-5m:
	.\hft-server.exe -feed FO -duration 5m

# Run HFT server for 5 minutes (Equity)
server-eq-5m:
	.\hft-server.exe -feed EQ -duration 5m

# Run HFT server with custom contract file
server-custom:
	.\hft-server.exe -feed FO -contracts ./data/tokens/BSE_EQD_CONTRACT.csv

# ================================================================================
# BENCHMARK - Statistics Only (No CSV output)
# ================================================================================

# Run benchmark with F&O feed (default)
benchmark:
	.\benchmark.exe

# Run with Equity feed
benchmark-eq:
	.\benchmark.exe -ip 239.1.2.5

# Run with custom duration (e.g., 60 seconds)
benchmark-60s:
	.\benchmark.exe -duration 60s

# Run with custom duration (5 minutes)
benchmark-5m:
	.\benchmark.exe -duration 5m

# ================================================================================
# DEVELOPMENT
# ================================================================================

# Development: Format code
fmt:
	go fmt ./...

# Development: Vet code
vet:
	go vet ./...

# Development: Run tests
test:
	go test -v ./...

# Development: Tidy dependencies
tidy:
	go mod tidy

# Help
help:
	@echo "BSE Go HFT System - Available targets:"
	@echo ""
	@echo "============================================================"
	@echo "BSE PORT CONFIGURATION:"
	@echo "============================================================"
	@echo "  EQ (Equity/Cash):       239.1.2.5:26001"
	@echo "  FO (F&O Derivatives):   239.1.2.5:26002"
	@echo ""
	@echo "============================================================"
	@echo "BUILD:"
	@echo "============================================================"
	@echo "  build           - Build all executables"
	@echo "  build-release   - Build optimized release binaries"
	@echo "  clean           - Remove build artifacts"
	@echo ""
	@echo "============================================================"
	@echo "GO RUN (Like Python - No Build Needed):"
	@echo "============================================================"
	@echo "  run             - go run hft-server F&O (port 26002)"
	@echo "  run-fo          - go run hft-server F&O (port 26002)"
	@echo "  run-eq          - go run hft-server EQ  (port 26001)"
	@echo "  run-5m          - go run hft-server F&O for 5 minutes"
	@echo "  run-eq-5m       - go run hft-server EQ for 5 minutes"
	@echo "  run-benchmark   - go run benchmark"
	@echo ""
	@echo "============================================================"
	@echo "HFT SERVER (Compiled EXE):"
	@echo "============================================================"
	@echo "  server          - F&O feed (port 26002)"
	@echo "  server-eq       - EQ feed  (port 26001)"
	@echo "  server-5m       - F&O for 5 minutes"
	@echo "  server-eq-5m    - EQ for 5 minutes"
	@echo ""
	@echo "============================================================"
	@echo "CUSTOM USAGE:"
	@echo "============================================================"
	@echo "  go run ./cmd/hft-server/main.go -feed FO -duration 5m"
	@echo "  go run ./cmd/hft-server/main.go -feed EQ -duration 5m"
	@echo "  .\hft-server.exe -feed FO -contracts ./data/contract.csv"
