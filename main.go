package main

import (
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
	"strings"
	"sync"

	"code.cloudfoundry.org/bytefmt"
	"github.com/pkg/browser"
)

var ctx struct {
	sync.Mutex
	pkgCount  int64
	totalSize int64
	pkgs      map[string]Package
	pkgmatch  *regexp.Regexp
}

type Package struct {
	idx     int64
	Size    int64
	Name    string
	Imports []string
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
	ctx.totalSize = 0
	ctx.pkgs = make(map[string]Package)
	ctx.pkgmatch = pkgmatch

	err = findImport(pkgName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	root := ctx.pkgs[pkgName]
	root.Size = ctx.totalSize
	ctx.pkgs[pkgName] = root

	err = renderGraph(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

}

func findImport(p string) error {
	if p == "C" {
		// C isn't really a package
		ctx.pkgCount++
		ctx.pkgs["C"] = Package{idx: ctx.pkgCount, Name: "C"}
	}
	if _, ok := ctx.pkgs[p]; ok {
		// seen this package before, skip it
		return nil
	}
	if strings.HasPrefix(p, "golang_org") {
		p = path.Join("vendor", p)
	}

	pkg, err := build.Import(p, "", 0)
	if err != nil {
		return err
	}
	ctx.pkgs[p] = analyze(pkg, p)
	for _, pkg := range ctx.pkgs[p].Imports {
		err = findImport(pkg)
		if err != nil {
			return err
		}
	}
	return nil
}

func analyze(pkg *build.Package, alias string) Package {
	imports := pkg.Imports
	var size int64
	info, err := os.Stat(pkg.PkgObj)
	if err == nil {
		size = info.Size()
		ctx.totalSize += size
	}
	ctx.pkgCount++
	return Package{idx: ctx.pkgCount, Name: alias, Size: size, Imports: imports}
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
	label := pkg.Name + " -> " + pkgImport.Name
	fmt.Fprintf(w, "\tN%d -> N%d [weight=1 tooltip=%q];\n", pkg.idx, pkgImport.idx, label)
}

func printNode(w io.Writer, pkg Package) {
	if pkg.Size > 0 {
		ratio := float64(pkg.Size) / float64(ctx.totalSize)
		// Scale font sizes from 8 to 32 based on percentage of flat frequency.
		// Use non linear growth to emphasize the size difference.
		baseFontSize, maxFontGrowth := 10, 24.0
		fontSize := baseFontSize
		if pkg.Size != ctx.totalSize {
			fontSize += int(math.Ceil(maxFontGrowth * math.Sqrt(ratio)))
		}

		label := fmt.Sprintf("%s\n%sB (%f%%)", pkg.Name, bytefmt.ByteSize(uint64(pkg.Size)), ratio*100)
		url := "/" + pkg.Name
		fmt.Fprintf(w, "\tN%d [label=%q,shape=box fontsize=%d URL=%q];\n", pkg.idx, label, fontSize, url)
	} else {
		fmt.Fprintf(w, "\tN%d [label=%q,shape=box];\n", pkg.idx, pkg.Name)
	}
}
