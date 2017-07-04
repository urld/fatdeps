// Fatdeps lets you analyze the sizes of a packages dependencies.
// The command starts a http server which serves interactive graphs.
//
// This command requires graphviz to be installed.
//
// Usage of fatdeps:
//  -http string
//    	HTTP Service address (default ":8080")
//
// Example:
//  fatdeps -http :9999 github.com/urld/fatdeps
//
// Visit https://github.com/urld/fatdeps for screenshots.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/build"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"code.cloudfoundry.org/bytefmt"
	"github.com/pkg/browser"
)

var ctx struct {
	sync.Mutex
	pkgCount    int64
	flatSize    int64
	symFlatSize int64
	cumSize     int64
	symsizes    bool
	pkgmatch    *regexp.Regexp
	pkgs        map[string]Package
}

type Package struct {
	idx        int64
	Size       int64
	CumSize    int64
	SymSize    int64
	CumSymSize int64
	Name       string
	Imports    []string
}

func main() {
	addr := flag.String("http", ":8080", "HTTP Service address")
	flag.Parse()
	if len(flag.Args()) != 1 {
		fmt.Println("exactly 1 package name required")
		os.Exit(2)
	}
	rootPkgName := flag.Arg(0)

	if strings.HasPrefix(*addr, ":") {
		browser.OpenURL("http://localhost" + *addr + "/" + rootPkgName)

	} else {
		browser.OpenURL("http://" + *addr + "/" + rootPkgName)
	}
	http.HandleFunc("/", handler)
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	pkgName := r.URL.Path[1:]
	match := r.URL.Query().Get("match")
	pkgmatch, err := regexp.Compile(match)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx.Lock()
	defer ctx.Unlock()
	ctx.pkgCount = 0
	ctx.flatSize = 0
	ctx.symFlatSize = 0
	ctx.cumSize = 0
	ctx.symsizes = strings.ToLower(r.URL.Query().Get("symsize")) == "true"
	ctx.pkgmatch = pkgmatch
	ctx.pkgs = make(map[string]Package)

	root, err := findImport(pkgName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	root.Size = ctx.flatSize
	ctx.cumSize = root.CumSize
	ctx.pkgs[pkgName] = root

	if ctx.symsizes {
		err := collectSymSizes(pkgName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	err = renderGraph(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

}

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

func renderGraph(w io.Writer) error {
	cmd := exec.Command("dot", "-Tsvg")
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	if err != nil {
		return err
	}

	fmt.Fprintf(in, "digraph \"\" {\n")
	for _, pkg := range ctx.pkgs {
		if ctx.pkgmatch.MatchString(pkg.Name) {
			printNode(in, pkg)
		}
	}
	for _, pkg := range ctx.pkgs {
		if pkg.Imports == nil {
			continue
		}
		if !ctx.pkgmatch.MatchString(pkg.Name) {
			continue
		}

		for _, pkgImport := range pkg.Imports {
			if ctx.pkgmatch.MatchString(pkgImport) {
				printEdge(in, pkg, ctx.pkgs[pkgImport])
			}
		}
	}
	fmt.Fprintf(in, "}\n")
	in.Close()
	_, err = io.Copy(w, out)
	if err != nil {
		return err
	}
	err = cmd.Wait()
	if err != nil {
		return err
	}
	return nil
}

func printEdge(w io.Writer, pkg, pkgImport Package) {
	tooltip := pkg.Name + " -> " + pkgImport.Name
	if pkg.Size > 0 {
		ratio := float64(pkgImport.CumSize) / float64(ctx.cumSize)
		baseWidth, maxWidthGrowth := 1.0, 6.0
		size := pkgImport.CumSize
		width := baseWidth
		if ctx.symsizes {
			// TODO
		}
		width += maxWidthGrowth * math.Sqrt(ratio)
		label := fmt.Sprintf(" %s", fmtSize(size))
		fmt.Fprintf(w, "\tN%d -> N%d [weight=1 penwidth=%f label=%q tooltip=%q labeltooltip=%q];\n",
			pkg.idx, pkgImport.idx, width, label, tooltip, tooltip)
	} else {
		fmt.Fprintf(w, "\tN%d -> N%d [weight=1 tooltip=%q];\n", pkg.idx, pkgImport.idx, tooltip)
	}
}

func printNode(w io.Writer, pkg Package) {
	if pkg.Size > 0 {
		ratio := float64(pkg.Size) / float64(ctx.flatSize)
		label := fmt.Sprintf("%s\nobjfile: %s (%.2f%%)", pkg.Name, fmtSize(pkg.Size), ratio*100)
		if ctx.symsizes {
			ratio = float64(pkg.SymSize) / float64(ctx.symFlatSize)
			label += fmt.Sprintf("\nsymsize: %s (%.2f%%)", fmtSize(pkg.SymSize), ratio*100)
		}

		// Scale font sizes from 8 to 32 based on percentage of flat frequency.
		// Use non linear growth to emphasize the size difference.
		baseFontSize, maxFontGrowth := 10, 24.0
		fontSize := baseFontSize
		if pkg.Size != ctx.flatSize {
			fontSize += int(math.Ceil(maxFontGrowth * math.Sqrt(ratio)))
		}

		url := "/" + pkg.Name
		fmt.Fprintf(w, "\tN%d [label=%q,shape=box fontsize=%d URL=%q];\n", pkg.idx, label, fontSize, url)
	} else {
		fmt.Fprintf(w, "\tN%d [label=%q,shape=box];\n", pkg.idx, pkg.Name)
	}
}

func fmtSize(size int64) string {
	if size < bytefmt.KILOBYTE {
		return bytefmt.ByteSize(uint64(size))
	}
	return bytefmt.ByteSize(uint64(size)) + "B"
}
