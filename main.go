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
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	"code.cloudfoundry.org/bytefmt"
	"github.com/pkg/browser"
)

var ctx struct {
	sync.Mutex
	pkgCount    int64
	flatSize    int64
	flatSymSize int64
	cumSize     int64
	cumSymSize  int64
	symsizes    bool
	pkgmatch    *regexp.Regexp
	symbols     map[string]int64
	pkgs        map[string]*Package
	pkgName     string
}

type Package struct {
	processed  bool
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
	ctx.flatSymSize = 0
	ctx.cumSize = 0
	ctx.symsizes = strings.ToLower(r.URL.Query().Get("symsize")) == "true"
	ctx.pkgmatch = pkgmatch
	ctx.symbols = make(map[string]int64)
	ctx.pkgs = make(map[string]*Package)
	ctx.pkgName = pkgName

	err = collectSymbols()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	root, err := findImport(pkgName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	analyzeRemainingSymbols(pkgName)
	calcCumSum(root)

	root.Size = ctx.flatSize
	ctx.cumSize = root.CumSize
	ctx.cumSymSize = root.CumSymSize

	err = renderGraph(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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
			printNode(in, *pkg)
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
				printEdge(in, *pkg, *ctx.pkgs[pkgImport])
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
		label := fmt.Sprintf(" obj: %s", fmtSize(pkgImport.CumSize))
		if ctx.symsizes {
			ratio = float64(pkgImport.CumSymSize) / float64(ctx.cumSymSize)
			label += fmt.Sprintf("\n sym: %s", fmtSize(pkgImport.CumSymSize))
		}

		baseWidth, maxWidthGrowth := 1.0, 6.0
		width := baseWidth
		width += maxWidthGrowth * math.Sqrt(ratio)

		fmt.Fprintf(w, "\tN%d -> N%d [weight=1 penwidth=%f label=%q tooltip=%q labeltooltip=%q fontsize=11];\n",
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
			ratio = float64(pkg.SymSize) / float64(ctx.flatSymSize)
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
