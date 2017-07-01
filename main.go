package main

import (
	"flag"
	"fmt"
	"go/build"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"

	"code.cloudfoundry.org/bytefmt"
	"github.com/pkg/browser"
)

var (
	pkgs      = make(map[string]Package)
	matchvar  = flag.String("match", ".*", "filter packages")
	filename  = flag.String("file", "", "dump to file instead of opening in browser")
	pkgmatch  *regexp.Regexp
	pkgCount  = int64(0)
	totalSize = int64(0)
)

type Package struct {
	idx     int64
	Size    int64
	Name    string
	Imports []string
}

func init() {
	flag.Parse()
	pkgmatch = regexp.MustCompile(*matchvar)
}

func main() {
	if len(flag.Args()) != 1 {
		fmt.Println("exactly 1 package name required")
		os.Exit(2)
	}
	rootPkgName := flag.Arg(0)

	findImport(rootPkgName)
	root := pkgs[rootPkgName]
	root.Size = totalSize
	pkgs[rootPkgName] = root

	var file *os.File
	var err error
	if *filename == "" {
		file, err = ioutil.TempFile(os.TempDir(), "fatdeps")
		check(err)
		defer os.Remove(file.Name())
		defer file.Close()
	} else {
		file, err = os.OpenFile(*filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		check(err)
		defer file.Close()
	}

	renderGraph(file)
	file.Close()
	browser.OpenFile(file.Name())

}

func findImport(p string) {
	if !pkgmatch.MatchString(p) {
		// doesn't match the filter, skip it
		return
	}
	if p == "C" {
		// C isn't really a package
		pkgCount++
		pkgs["C"] = Package{idx: pkgCount, Name: "C"}
	}
	if _, ok := pkgs[p]; ok {
		// seen this package before, skip it
		return
	}
	if strings.HasPrefix(p, "golang_org") {
		p = path.Join("vendor", p)
	}

	pkg, err := build.Import(p, "", 0)
	if err != nil {
		log.Fatal(err)
	}
	pkgs[p] = analyze(pkg, p)
	for _, pkg := range pkgs[p].Imports {
		findImport(pkg)
	}
}

func analyze(pkg *build.Package, alias string) Package {
	imports := filter(pkg.Imports)
	var size int64
	info, err := os.Stat(pkg.PkgObj)
	if err == nil {
		size = info.Size()
		totalSize += size
	}
	pkgCount++
	return Package{idx: pkgCount, Name: alias, Size: size, Imports: imports}
}

func filter(s []string) []string {
	var r []string
	for _, v := range s {
		if pkgmatch.MatchString(v) {
			r = append(r, v)
		}
	}
	return r
}

func renderGraph(w io.Writer) {
	cmd := exec.Command("dot", "-Tsvg")
	in, err := cmd.StdinPipe()
	check(err)
	out, err := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr
	check(cmd.Start())

	fmt.Fprintf(in, "digraph \"\" {\n")
	for _, pkg := range pkgs {
		printNode(in, pkg)
	}
	for _, pkg := range pkgs {
		if pkg.Imports == nil {
			continue
		}
		for _, pkgImport := range pkg.Imports {
			printEdge(in, pkg, pkgs[pkgImport])
		}
	}
	fmt.Fprintf(in, "}\n")
	in.Close()
	io.Copy(w, out)
	check(cmd.Wait())
}

func printEdge(w io.Writer, pkg, pkgImport Package) {
	label := pkg.Name + " -> " + pkgImport.Name
	fmt.Fprintf(w, "\tN%d -> N%d [weight=1 tooltip=%q];\n", pkg.idx, pkgImport.idx, label)
}

func printNode(w io.Writer, pkg Package) {
	if pkg.Size > 0 {
		ratio := float64(pkg.Size) / float64(totalSize)
		// Scale font sizes from 8 to 32 based on percentage of flat frequency.
		// Use non linear growth to emphasize the size difference.
		baseFontSize, maxFontGrowth := 10, 24.0
		fontSize := baseFontSize
		if pkg.Size != totalSize {
			fontSize += int(math.Ceil(maxFontGrowth * math.Sqrt(ratio)))
		}

		label := fmt.Sprintf("%s\n%s (%f%%)", pkg.Name, bytefmt.ByteSize(uint64(pkg.Size)), ratio*100)
		fmt.Fprintf(w, "\tN%d [label=%q,shape=box fontsize=%d];\n", pkg.idx, label, fontSize)
	} else {
		fmt.Fprintf(w, "\tN%d [label=%q,shape=box];\n", pkg.idx, pkg.Name)
	}
}

func check(err error) {

	if err != nil {
		log.Fatal(err)
	}
}
