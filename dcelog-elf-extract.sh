#!/bin/bash

# Exit on any error
set -e

# Check arguments
if [[ $# -ne 2 ]]; then
    echo "Usage: $0 <URL-to-full_linux_for_tegra.tbz2> <pushtag>"
    exit 1
fi

# Input arguments
URL="$1"
PUSTAG="$2"

# Temporary working directory
WORKDIR=$(mktemp -d)

echo "Using temp directory: $WORKDIR"

# Function to extract build ID from ELF file
extract_build_id_from_elf() {
    local elf_file="$1"
    local build_id
    
    build_id=$(readelf -n "$elf_file" 2>/dev/null | grep "Build ID:" | sed 's/.*Build ID: //' | tr -d ' ')
    
    if [[ -n "$build_id" ]]; then
        echo "$build_id"
    else
        echo "unknown"
    fi
}

# Function to extract build ID from binary file
extract_build_id_from_bin() {
    local bin_file="$1"
    local build_id
    
    # Use Python to extract build ID from binary
    build_id=$(python3 -c "
import struct
import binascii
import sys

try:
    with open('$bin_file', 'rb') as f:
        data = f.read()
    
    # Search for ELF note header pattern: name_size=4, desc_size=20, type=3, 'GNU\0'
    note_pattern = struct.pack('<III4s', 4, 20, 3, b'GNU\0')
    offset = data.find(note_pattern)
    
    if offset != -1:
        # Build ID starts 16 bytes after the note header
        build_id_offset = offset + 16
        build_id_bytes = data[build_id_offset:build_id_offset + 20]
        print(binascii.hexlify(build_id_bytes).decode())
    else:
        print('unknown')
except Exception as e:
    print('unknown')
" 2>/dev/null)
    
    if [[ -n "$build_id" && "$build_id" != "unknown" ]]; then
        echo "$build_id"
    else
        echo "unknown"
    fi
}

# Download full_linux_for_tegra.tbz2
echo "Downloading full_linux_for_tegra.tbz2 from: $URL"
curl -L "$URL/full_linux_for_tegra.tbz2" -o "$WORKDIR/full_linux_for_tegra.tbz2"

# Extract full_linux_for_tegra.tbz2
echo "Extracting full_linux_for_tegra.tbz2..."
tar -xjf "$WORKDIR/full_linux_for_tegra.tbz2" -C "$WORKDIR"

### Extract host_overlay_deployed.tbz2 and find ELF

# Locate host_overlay_deployed.tbz2
B_TBZ2=$(find "$WORKDIR" -name "host_overlay_deployed.tbz2" | head -n 1)

if [[ -z "$B_TBZ2" ]]; then
    echo "Error: host_overlay_deployed.tbz2 not found."
    exit 1
fi

echo "Found host_overlay_deployed.tbz2: $B_TBZ2"

# Extract it
mkdir "$WORKDIR/host_overlay"
tar -xjf "$B_TBZ2" -C "$WORKDIR/host_overlay"

# Find the ELF
C_ELF=$(find "$WORKDIR/host_overlay" -name "display-t234-dce-log.elf" | head -n 1)

if [[ -z "$C_ELF" ]]; then
    echo "Error: display-t234-dce-log.elf not found."
    exit 1
fi

echo "Found display-t234-dce-log.elf: $C_ELF"

# Extract build ID from ELF
echo "Extracting build ID from ELF..."
ELF_BUILD_ID=$(extract_build_id_from_elf "$C_ELF")
echo "ELF Build ID: $ELF_BUILD_ID"

# ELF output path with build ID
ELF_OUTPUT="./display-t234-dce-log.elf__${PUSTAG}__${ELF_BUILD_ID}"

# Copy and rename ELF
cp "$C_ELF" "$ELF_OUTPUT"
echo "ELF extracted and saved to: $ELF_OUTPUT"

### Extract flashlight_partner.tbz2 and find BIN

# Locate flashlight_partner.tbz2
F_TBZ2=$(find "$WORKDIR" -name "flashlight_partner.tbz2" | head -n 1)

if [[ -z "$F_TBZ2" ]]; then
    echo "Error: flashlight_partner.tbz2 not found."
    exit 1
fi

echo "Found flashlight_partner.tbz2: $F_TBZ2"

# Extract it
mkdir "$WORKDIR/flashlight_partner"
tar -xjf "$F_TBZ2" -C "$WORKDIR/flashlight_partner"

# Find the BIN
D_BIN=$(find "$WORKDIR/flashlight_partner" -name "display-t234-dce.bin" | head -n 1)

if [[ -z "$D_BIN" ]]; then
    echo "Error: display-t234-dce.bin not found."
    exit 1
fi

echo "Found display-t234-dce.bin: $D_BIN"

# Compute MD5 sum
echo "Computing MD5 sum..."
MD5=$(md5sum "$D_BIN" | awk '{ print $1 }')
echo "BIN MD5: $MD5"

# Extract build ID from BIN
echo "Extracting build ID from BIN..."
BIN_BUILD_ID=$(extract_build_id_from_bin "$D_BIN")
echo "BIN Build ID: $BIN_BUILD_ID"

# BIN output path with MD5 and build ID
BIN_OUTPUT="./display-t234-dce.bin__${PUSTAG}__${MD5}__${BIN_BUILD_ID}"

# Copy and rename BIN
cp "$D_BIN" "$BIN_OUTPUT"
echo "BIN extracted and saved to: $BIN_OUTPUT"

# Verify build IDs match (they should be the same)
if [[ "$ELF_BUILD_ID" == "$BIN_BUILD_ID" && "$ELF_BUILD_ID" != "unknown" ]]; then
    echo "✅ Build ID verification: ELF and BIN build IDs match ($ELF_BUILD_ID)"
elif [[ "$ELF_BUILD_ID" != "unknown" && "$BIN_BUILD_ID" != "unknown" ]]; then
    echo "⚠️  Build ID warning: ELF ($ELF_BUILD_ID) and BIN ($BIN_BUILD_ID) build IDs differ"
else
    echo "ℹ️  Build ID info: One or both build IDs could not be extracted"
fi

# Summary
echo ""
echo "=== EXTRACTION SUMMARY ==="
echo "Push Tag: $PUSTAG"
echo "ELF File: $ELF_OUTPUT"
echo "BIN File: $BIN_OUTPUT"
echo "ELF Build ID: $ELF_BUILD_ID"
echo "BIN Build ID: $BIN_BUILD_ID"
echo "BIN MD5: $MD5"
echo "=========================="

# Clean up
rm -rf "$WORKDIR"
echo "Cleanup completed."
