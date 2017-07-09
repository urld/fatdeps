package main

import (
	"bufio"
	"fmt"
	"go/build"
	"os/exec"
	"path"
	"strconv"
	"strings"
)

func collectSymSizes(pkgName string) error {
	builtPkg, err := build.Import(pkgName, "", 0)
	if err != nil {
		return err
	}
	if !builtPkg.IsCommand() {
		return fmt.Errorf("symbol sizes can only be determined from complete binaries (commands)")
	}
	binary := path.Join(builtPkg.BinDir, path.Base(pkgName))

	cmd := exec.Command("go", "tool", "nm", "-size", binary)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	symsizes := make(map[string]int64)
	s := bufio.NewScanner(out)
	// collect all symbols
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "    ") {
			continue
		}
		fields := strings.Fields(line)
		size, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return err
		}
		if size > 0 {
			symname := fields[3]
			symsizes[symname] += size
			ctx.symFlatSize += size
		}
	}

	if err := cmd.Wait(); err != nil {
		return err
	}

	// assign symbols to pkgs
	for pkgKey, pkgVal := range ctx.pkgs {
		for symKey, symSize := range symsizes {
			if strings.HasPrefix(symKey, pkgKey+".") {
				pkgVal.SymSize += symSize
				delete(symsizes, symKey)
			}
		}
		ctx.pkgs[pkgKey] = pkgVal
	}

	// add remaining symbols to runtime pkg
	var mainSize int64
	var remainingSize int64
	for symKey, symSize := range symsizes {
		if strings.HasPrefix(symKey, "main") {
			mainSize += symSize
		}
		remainingSize += symSize
	}
	rtPkg := ctx.pkgs["runtime"]
	rtPkg.SymSize += remainingSize
	ctx.pkgs["runtime"] = rtPkg

	mainPkg := ctx.pkgs[pkgName]
	mainPkg.SymSize += mainSize
	ctx.pkgs[pkgName] = mainPkg

	return nil
}
