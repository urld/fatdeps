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

func collectSymbols() error {
	if !ctx.symsizes {
		return nil
	}

	// check if pkgName is command first
	builtPkg, err := build.Import(ctx.pkgName, "", 0)
	if err != nil {
		return err
	}
	if !builtPkg.IsCommand() {
		fmt.Println("Warning: symbol sizes can only be determined for commands. (" + ctx.pkgName + ")")
		ctx.symsizes = false
		return nil
	}

	// call 'go tool nm' to collect symbol sizes
	binary := path.Join(builtPkg.BinDir, path.Base(ctx.pkgName))
	cmd := exec.Command("go", "tool", "nm", "-size", binary)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	// parse output of 'go tool nm'
	s := bufio.NewScanner(out)
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
			ctx.symbols[symname] += size
			ctx.flatSymSize += size
		}
	}

	return cmd.Wait()
}

func analyzeSymbols(pkg *Package) {
	for sym, size := range ctx.symbols {
		if strings.HasPrefix(sym, pkg.Name+".") {
			pkg.SymSize += size
			delete(ctx.symbols, sym)
		}
	}
}

func analyzeRemainingSymbols(mainPkgName string) {
	var mainSize int64
	var remainingSize int64
	for sym, size := range ctx.symbols {
		if strings.HasPrefix(sym, "main") {
			mainSize += size
		}
		remainingSize += size
	}

	ctx.pkgs[mainPkgName].SymSize += mainSize
	ctx.pkgs["runtime"].SymSize += remainingSize

}
