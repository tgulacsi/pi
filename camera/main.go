package main

import (
	"bufio"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	workDir    = "/var/www"
	httpPrefix = ""
	remote     = ""
)

func main() {
	flagHttp := flag.String("http", ":9001", "listen on")
	flagPrefix := flag.String("prefix", httpPrefix, "HTTP prefix to strip")
	flagDir := flag.String("dir", workDir, "directory to save images to")
	flagRemote := flag.String("remote", remote, "RPi user@host (e.g. pi@192.168.1.103)")
	flag.Parse()

	workDir = *flagDir
	httpPrefix = *flagPrefix
	remote = *flagRemote

	//http.Handle("/", http.StripPrefix(*flagPrefix, http.HandlerFunc(index)))
	http.HandleFunc("/", index)

	log.Printf("start listening on %s", *flagHttp)
	log.Fatal(http.ListenAndServe(*flagHttp, nil))
}

func index(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s", r)
	if strings.HasSuffix(r.URL.Path, ".jpg") {
		i := strings.LastIndex(r.URL.Path, "/")
		if i >= 0 {
			http.ServeFile(w, r, filepath.Join(workDir, r.URL.Path[i+1:]))
			return
		}
	}

	q := r.URL.Query()
	todo := q.Get("command")
	if todo == "" {
		todo = "capture"
	}
	widthS := q.Get("width")
	if widthS == "" {
		widthS = "1024"
	}
	heightS := q.Get("height")
	if heightS == "" {
		heightS = "768"
	}
	drc := q.Get("drc")
	if drc == "" {
		drc = "low"
	}
	ex := q.Get("ex")
	if ex == "" {
		ex = "auto"
	}
	opts := options{Drc: drc, Ex: ex}
	opts.Width, _ = strconv.Atoi(widthS)
	opts.Height, _ = strconv.Atoi(heightS)

	var message string
	switch todo {
	case "capture":
		fn, err := saveImage(workDir, opts)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		message = "Image has been saved saved to " + fn
		link := filepath.Join(filepath.Dir(fn), "last.jpg")
		os.Remove(link)
		os.Symlink(fn, link)
		// continue with the default page
	default:
		message = "unknown command " + todo
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, `<!DOCTYPE html>
<html><head><meta http-equiv="refresh" content="60" /><title>Camera</title></head>
<body>
<p>`+message+`</p>
<p><img src="`+httpPrefix+`last.jpg" width="640" height="480" alt="last.jpg" /></p>
`)
	dh, err := os.Open(workDir)
	if err != nil {
		io.WriteString(w, "<pre>"+err.Error()+"</pre>")
	} else {
		names, err := dh.Readdirnames(-1)
		dh.Close()
		if err != nil {
			io.WriteString(w, "<pre>"+err.Error()+"</pre>")
		} else {
			io.WriteString(w, "<ul>\n")
			j := 0
			for i := len(names) - 1; i >= 0; i-- {
				if !strings.HasPrefix(names[i], "img-") || !strings.HasSuffix(names[i], ".jpg") {
					continue
				}
				if j > 1000 {
					os.Remove(filepath.Join(workDir, names[i]))
				} else {
					io.WriteString(w, "<li><a href=\""+httpPrefix+names[i]+"\">"+names[i]+"</a>\n")
				}
				j++

			}
			io.WriteString(w, "</ul>\n")
		}
	}
	io.WriteString(w, `</body></html>`)
}

type options struct {
	Drc, Ex       string
	Width, Height int
}

func writeImage(w io.Writer, opts options) error {
	if opts.Width <= 0 {
		opts.Width = 1024
	}
	if opts.Height <= 0 || opts.Width < opts.Height*4/3 {
		opts.Height = opts.Width * 3 / 4
	}
	cmdS := "raspistill"
	args := []string{
		"-ex", opts.Ex,
		"-drc", opts.Drc,
		"-q", "80",
		"-w", strconv.Itoa(opts.Width),
		"-h", strconv.Itoa(opts.Height),
		"-e", "jpg",
		"--nopreview",
		"-t", "1",
		"-o", "-",
	}
	if remote != "" {
		args = append([]string{remote, cmdS}, args...)
		cmdS = "ssh"
	}
	cmd := exec.Command(cmdS, args...)
	log.Printf("calling %s %s", cmd.Path, cmd.Args)
	bw := bufio.NewWriter(w)
	cmd.Stderr = os.Stderr
	cmd.Stdout = bw
	if err := cmd.Run(); err != nil {
		return err
	}
	return bw.Flush()
}

func saveImage(destDir string, opts options) (string, error) {
	fh, err := os.Create(filepath.Join(destDir,
		"img-"+time.Now().Format("20060102_150405")+".jpg"))
	if err != nil {
		return "", err
	}
	fn := fh.Name()
	if err = writeImage(fh, opts); err != nil {
		return fn, err
	}
	return fn, fh.Close()
}
