# BSE HFT C++ Makefile
# Supports: MinGW-w64, GCC, Clang

CXX ?= g++
CXXFLAGS = -std=c++17 -O3 -march=native -mtune=native -flto -ffast-math
CXXFLAGS += -Wall -Wextra -DNDEBUG
CXXFLAGS += -I include

# Windows-specific
ifeq ($(OS),Windows_NT)
    LDFLAGS = -lws2_32 -static
    EXE = bse-hft.exe
    RM = del /q
    MKDIR = if not exist build mkdir build
else
    LDFLAGS = -pthread
    EXE = bse-hft
    RM = rm -f
    MKDIR = mkdir -p build
endif

# Source files
SRCS = src/main.cpp
OBJS = $(SRCS:.cpp=.o)

# Target
TARGET = build/$(EXE)

.PHONY: all clean release debug

all: release

release: CXXFLAGS += -O3 -DNDEBUG
release: $(TARGET)

debug: CXXFLAGS += -g -O0 -fsanitize=address
debug: $(TARGET)

$(TARGET): $(SRCS)
	@$(MKDIR)
	$(CXX) $(CXXFLAGS) $(SRCS) -o $(TARGET) $(LDFLAGS)
	@echo "Build complete: $(TARGET)"

clean:
	$(RM) build\*.exe build\*.obj 2>nul

run: $(TARGET)
	cd build && ./$(EXE)

# Show configuration
info:
	@echo "Compiler: $(CXX)"
	@echo "Flags: $(CXXFLAGS)"
	@echo "Target: $(TARGET)"
