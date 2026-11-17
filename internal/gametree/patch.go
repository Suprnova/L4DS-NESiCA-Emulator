// Package gametree builds private copies of the game ("trees") and patches their binaries, so
// the installed game is never written to.
//
// A server tree runs the arcade build as a dedicated server through cmd/srcds-stub; a client
// tree runs a copy of the game. Patches replace a few bytes at a virtual address of a DLL in the
// tree's bin folder. Each one checks the bytes it expects first and never touches bytes the
// loader rewrites when it rebases the DLL, so every patch can be reverted exactly.
package gametree

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Patch replaces Original with Patched at virtual address VA of a DLL in a tree's bin folder.
type Patch struct {
	DLL      string
	VA       uint32
	Original []byte
	Patched  []byte
	Why      string
}

// ServerPatches make the arcade build run as a dedicated server.
var ServerPatches = []Patch{
	{"MaterialSystem.dll", 0x1009ed6b, h("36"), h("37"),
		`mat_monitorgamma's help text says "1.7 for LCD" like engine.dll's; the dedicated app treats the mismatch as fatal`},
	{"MaterialSystem.dll", 0x1000cec9, h("7574"), h("eb74"),
		"EndRenderTargetAllocation skips the shader device: the 2016 MaterialSystem calls IShaderDevice_Support methods the 2013 shaderapiempty.dll lacks"},
	{"MaterialSystem.dll", 0x1000e425, h("83783000740e"), h("b001c2040090"),
		"ReleaseResources skips the shader device, for the same reason"},
	{"MaterialSystem.dll", 0x1000e458, h("83783000740e"), h("b0015dc20400"),
		"ReacquireResources skips the shader device, for the same reason"},
	{"engine.dll", 0x1001c9e0, h("e8"), h("c3"),
		"S_ExtraUpdate returns at once: a dedicated server has no audio device, and Taito's S_ExtraUpdate calls S_Update without checking"},
	{"engine.dll", 0x101b7999, h("740f"), h("eb0f"),
		"NET_SendPacket always uses the socket, so replies to the game's 127.0.0.1 reach it from a server bound to 127.0.0.2"},
	{"engine.dll", 0x101984c7, h("751f"), h("eb1f"),
		"HostState_NewGame always starts the level; Taito's hands it to the client UI, so map and lobby reservations never load"},
}

// ClientPatches let a copy of the game run beside the installed one and take its card from the
// console: `change_debug_dummyCardID <card>` puts a card on the reader and
// `change_debug_dummyCardID 0` takes it off. A real card reader still works.
var ClientPatches = []Patch{
	{"launcher.dll", 0x1000575e, h("75cc"), h("ebcc"),
		"skip the hl2_singleton_mutex check, so the copy starts while another game runs (-multirun would, but also bypasses NESYS)"},
	{"GameUI.dll", 0x100e2ec8, bytes.Repeat(h("cc"), 36),
		h("8b01ff90a400000084c00f8514d9f5ffe800000000588b8097031e008b402ce92ed9f5ff"),
		"code in int3 padding: ask the card reader, and without one read the cvar debug_dummyCardID (position-independent, as the DLL is rebased)"},
	{"GameUI.dll", 0x100407c8, h("8b018b90a4"), h("e9fb260a00"),
		"the attract screen's card check jumps to that code instead of testing for -simulateNESICA"},
	{"GameUI.dll", 0x1004c430, h("ff15"), h("eb22"),
		"reading the card no longer copies -simulateNESICA's value into debug_dummyCardID"},
	{"GameUI.dll", 0x101db853, h("00400000"), h("00000800"),
		"change_debug_dummyCardID is a release command instead of a cheat; debug_dummyCardID itself is hidden from the console"},
	{"engine.dll", 0x101ec647, h("7404"), h("eb04"),
		"a card on the reader no longer stops the startup and attract movies; input still does"},
}

// ErrUnexpectedBytes reports that a patch site holds neither the original nor the patched bytes.
var ErrUnexpectedBytes = errors.New("gametree: unexpected bytes at patch site")

// Apply applies patches to the DLLs in tree's bin folder. A patch already applied is left alone.
func Apply(tree string, patches []Patch) error {
	return rewrite(tree, patches, func(p Patch) (from, to []byte) { return p.Original, p.Patched })
}

// Revert restores the original bytes of patches in tree's bin folder. A patch not applied is
// left alone.
func Revert(tree string, patches []Patch) error {
	return rewrite(tree, patches, func(p Patch) (from, to []byte) { return p.Patched, p.Original })
}

func rewrite(tree string, patches []Patch, direction func(Patch) (from, to []byte)) error {
	byDLL := map[string][]Patch{}
	var order []string
	for _, p := range patches {
		if _, seen := byDLL[p.DLL]; !seen {
			order = append(order, p.DLL)
		}
		byDLL[p.DLL] = append(byDLL[p.DLL], p)
	}

	for _, dll := range order {
		path := filepath.Join(tree, "bin", dll)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		image, err := parseImage(data)
		if err != nil {
			return fmt.Errorf("%s: %w", dll, err)
		}
		for _, p := range byDLL[dll] {
			from, to := direction(p)
			if err := image.replace(p.VA, from, to); err != nil {
				return fmt.Errorf("%s %#x: %w", dll, p.VA, err)
			}
		}
		if err := os.WriteFile(path, image.data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// image is a PE file's bytes with what is needed to patch them: where each section lies and
// which bytes the loader rewrites when it rebases the image.
type image struct {
	data      []byte
	base      uint32
	sections  []pe.SectionHeader
	relocated map[uint32]bool
}

func parseImage(data []byte) (*image, error) {
	file, err := pe.NewFile(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	header, ok := file.OptionalHeader.(*pe.OptionalHeader32)
	if !ok {
		return nil, errors.New("not a 32-bit image")
	}

	img := &image{data: data, base: uint32(header.ImageBase)}
	for _, section := range file.Sections {
		img.sections = append(img.sections, section.SectionHeader)
	}

	directory := header.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_BASERELOC]
	if directory.Size > 0 {
		offset, err := img.offset(img.base + directory.VirtualAddress)
		if err != nil {
			return nil, err
		}
		img.relocated = relocatedBytes(data[offset : offset+int(directory.Size)])
	}
	return img, nil
}

// relocatedBytes lists the RVAs of every byte that a base relocation table's HIGHLOW entries
// make the loader rewrite.
func relocatedBytes(table []byte) map[uint32]bool {
	const highLow = 3
	relocated := map[uint32]bool{}
	for len(table) >= 8 {
		page := binary.LittleEndian.Uint32(table)
		size := binary.LittleEndian.Uint32(table[4:])
		if size < 8 || int(size) > len(table) {
			break
		}
		for entry := table[8:size]; len(entry) >= 2; entry = entry[2:] {
			value := binary.LittleEndian.Uint16(entry)
			if value>>12 == highLow {
				rva := page + uint32(value&0xfff)
				for k := range uint32(4) {
					relocated[rva+k] = true
				}
			}
		}
		table = table[size:]
	}
	return relocated
}

// offset returns the file offset of virtual address va.
func (img *image) offset(va uint32) (int, error) {
	rva := va - img.base
	for _, s := range img.sections {
		if rva >= s.VirtualAddress && rva-s.VirtualAddress < s.Size {
			return int(s.Offset + rva - s.VirtualAddress), nil
		}
	}
	return 0, fmt.Errorf("gametree: address %#x is not in the file", va)
}

// replace swaps from for to at va. Bytes that already read to are left alone, and the swap is
// refused when the site holds neither or overlaps relocated bytes.
func (img *image) replace(va uint32, from, to []byte) error {
	if len(from) != len(to) {
		return errors.New("gametree: patch lengths differ")
	}
	for rva := va - img.base; rva < va-img.base+uint32(len(to)); rva++ {
		if img.relocated[rva] {
			return fmt.Errorf("gametree: patch overlaps relocated byte at %#x", img.base+rva)
		}
	}

	offset, err := img.offset(va)
	if err != nil {
		return err
	}
	site := img.data[offset : offset+len(to)]
	switch {
	case bytes.Equal(site, to):
		return nil
	case bytes.Equal(site, from):
		copy(site, to)
		return nil
	default:
		return fmt.Errorf("%w: found %x, expected %x", ErrUnexpectedBytes, site, from)
	}
}

// h decodes a hex literal of the patch tables.
func h(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}
