package main

import (
	"go/build"
	"os"
	"path"
	"strings"
)

func findImport(p string) (*Package, error) {
	if p == "C" {
		// C isn't really a package
		ctx.pkgCount++
		ctx.pkgs["C"] = &Package{idx: ctx.pkgCount, Name: "C"}
	}
	if pkg, ok := ctx.pkgs[p]; ok {
		// seen this package before, skip it
		return pkg, nil
	}
	if strings.HasPrefix(p, "golang_org") {
		p = path.Join("vendor", p)
	}

	builtPkg, err := build.Import(p, "", 0)
	if err != nil {
		return new(Package), err
	}
	pkg := analyze(builtPkg, p)
	ctx.pkgs[p] = pkg
	for _, pkgImport := range pkg.Imports {
		_, err := findImport(pkgImport)
		if err != nil {
			return pkg, err
		}
	}
	ctx.pkgs[p] = pkg
	return pkg, nil
}

func calcCumSum(pkg *Package) {
	if pkg.processed {
		return
	}
	pkg.processed = true
	pkg.CumSize = pkg.Size
	pkg.CumSymSize = pkg.SymSize
	for _, pkgImport := range pkg.Imports {
		importPkg, _ := ctx.pkgs[pkgImport]
		calcCumSum(importPkg)
		pkg.CumSize += importPkg.CumSize
		pkg.CumSymSize += importPkg.CumSymSize
	}
}

func (p *Package) store() {
	ctx.pkgs[p.Name] = p
}

func analyze(buildPkg *build.Package, alias string) *Package {
	imports := buildPkg.Imports
	var size int64
	info, err := os.Stat(buildPkg.PkgObj)
	if err == nil {
		size = info.Size()
		ctx.flatSize += size
	}
	ctx.pkgCount++
	pkg := &Package{idx: ctx.pkgCount, Name: alias, Size: size, Imports: imports}
	analyzeSymbols(pkg)
	return pkg
}
