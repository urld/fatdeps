package main

import (
	"go/build"
	"os"
	"path"
	"strings"
)

func findImport(p string) (Package, error) {
	if p == "C" {
		// C isn't really a package
		ctx.pkgCount++
		ctx.pkgs["C"] = Package{idx: ctx.pkgCount, Name: "C"}
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
		return Package{}, err
	}
	pkg := analyze(builtPkg, p)
	ctx.pkgs[p] = pkg
	for _, pkgImport := range pkg.Imports {
		importPkg, err := findImport(pkgImport)
		if err != nil {
			return pkg, err
		}
		pkg.CumSize += importPkg.CumSize
	}
	ctx.pkgs[p] = pkg
	return pkg, nil
}

func analyze(pkg *build.Package, alias string) Package {
	imports := pkg.Imports
	var size int64
	info, err := os.Stat(pkg.PkgObj)
	if err == nil {
		size = info.Size()
		ctx.flatSize += size
	}
	ctx.pkgCount++
	return Package{idx: ctx.pkgCount, Name: alias, Size: size, CumSize: size, Imports: imports}
}
