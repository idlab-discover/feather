package main

import (
	"debug/elf"
	"fmt"
	"github.com/go-delve/delve/pkg/proc"
	"os"
	"runtime"
	"sort"
	"strings"
)

func main() {
	// Use delve to decode the DWARF section
	binInfo := proc.NewBinaryInfo(runtime.GOOS, runtime.GOARCH)
	if err := binInfo.AddImage(os.Args[1], 0); err != nil {
		panic(err)
	}

	// Make a list of unique packages
	pkgs := make([]string, 0, len(binInfo.PackageMap))
	for _, fullPkgs := range binInfo.PackageMap {
		for _, fullPkg := range fullPkgs {
			exists := false
			for _, pkg := range pkgs {
				if fullPkg == pkg {
					exists = true
					break
				}
			}
			if !exists {
				pkgs = append(pkgs, fullPkg)
			}
		}
	}
	// Sort them for a nice output
	sort.Strings(pkgs)

	// Parse the ELF file ourselfs
	elfFile, err := elf.Open(os.Args[1])
	if err != nil {
		panic(err)
	}

	// Get the symbol table
	symbols, err := elfFile.Symbols()
	if err != nil {
		panic(err)
	}

	usage := make(map[string]int)
	for _, sym := range symbols {
		if sym.Section == elf.SHN_UNDEF || sym.Section >= elf.SectionIndex(len(elfFile.Sections)) {
			continue
		}
		symPkg := ""
		for _, pkg := range pkgs {
			if strings.HasPrefix(sym.Name, pkg) {
				symPkg = pkg
				break
			}
		}
		// Symbol doesn't belong to a known package
		if symPkg == "" {
			continue
		}
		usage[symPkg] += int(sym.Size)
	}

	total := 0
	for _, pkg := range pkgs {
		total += usage[pkg]
	}

	keys := make([]string, 0, len(pkgs))
	for key := range usage {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		return usage[keys[i]] < usage[keys[j]]
	})

	for _, key := range keys {
		size, exists := usage[key]
		if !exists {
			continue
		}
		if len(key) > 20 {
			key = "..." + key[len(key)-20+3:]
		}
		fmt.Printf("%20s: %6d bytes\n", key, size)
	}
}
