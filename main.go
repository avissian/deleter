package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/alecthomas/kingpin/v2"
	"github.com/avissian/banner/tlo_config"
	"github.com/avissian/go-qbittorrent/qbt"
	"github.com/pterm/pterm"
)

func main() {
	configPath := kingpin.Arg("path", "Путь к файлу конфига ТЛО").Required().File()

	if len(os.Args) < 2 {
		os.Args = append(os.Args, "--help")
	}
	kingpin.Parse()
	var paths []string

	file, err := os.Open("paths.txt")
	if err != nil {
		log.Panicln(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		path := scanner.Text()
		if strings.TrimSpace(path) != "" {
			paths = append(paths, path)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	var wait sync.WaitGroup
	wait.Add(2)
	var localFiles []string
	var torrentFiles []string
	multi := pterm.DefaultMultiPrinter
	multi.Start()
	go getLocalFiles(paths, &localFiles, &multi, &wait)
	go getTorrentFiles((*configPath).Name(), &torrentFiles, &multi, &wait)
	wait.Wait()

	list := compare(localFiles, torrentFiles, &multi)

	// дебажно срём во все файлы
	/*	slices.Sort(localFiles)
		slices.Sort(torrentFiles)
		slices.Sort(list)

		outFile, err := os.Create("localFiles.txt")
		if err != nil {
			log.Panicln(err)
		}
		defer outFile.Close()
		for _, file := range localFiles {
			outFile.WriteString(file + "\n")
		}

		outFile, err = os.Create("torrentFiles.txt")
		if err != nil {
			log.Panicln(err)
		}
		defer outFile.Close()
		for _, file := range torrentFiles {
			outFile.WriteString(file + "\n")
		}*/
	// end debug

	outFile, err := os.Create("out.txt")
	if err != nil {
		log.Panicln(err)
	}
	defer outFile.Close()
	for _, file := range list {
		outFile.WriteString(file + "\n")
	}

}

func replacer(s string) string {
	if runtime.GOOS == "windows" {
		return strings.Replace(s, "/", "\\", -1)
	}
	return s
}

func Connect(user string, pass string, server string, port uint32, SSL bool) (client *qbt.Client) {
	scheme := "http"
	if SSL {
		scheme = "https"
	}
	lo := qbt.LoginOptions{Username: user, Password: pass}
	client = qbt.NewClient(fmt.Sprintf("%s://%s:%d", scheme, server, port))
	if err := client.Login(lo); err != nil {
		log.Panicln(err)
	}
	return
}

func getLocalFiles(paths []string, files *[]string, multi *pterm.MultiPrinter, wait *sync.WaitGroup) {
	defer (*wait).Done()
	pb1, _ := pterm.DefaultProgressbar.WithTotal(len(paths)).WithWriter((*multi).NewWriter()).Start("Get FS files list")

	var wg sync.WaitGroup
	c := make(chan []string, len(paths))

	wg.Add(len(paths))
	go func() {
		wg.Wait()
		close(c)
	}()
	for _, v := range paths {
		go ls(v, c, &wg)
	}
	idx := 0
	for val := range c {
		idx++
		*files = append(*files, val...)
		pb1.Increment()
	}

	pb1.Increment().Stop()
}

func ls(path string, c chan []string, wg *sync.WaitGroup) {
	defer (*wg).Done()
	var res []string
	err := filepath.WalkDir(path, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			res = append(res, path)
		}
		return nil
	})
	if err != nil {
		log.Panicln(err)
	}
	c <- res
}

func getTorrentFiles(tloPath string, files *[]string, multi *pterm.MultiPrinter, wait *sync.WaitGroup) {
	defer (*wait).Done()

	var tlo tlo_config.ConfigT
	err := tlo.Load(tloPath)
	if err != nil {
		log.Panicln(err)
	}

	pterm.DefaultBasicText.WithWriter((*multi).NewWriter()).Printf("Torrent clients: %d", len(tlo.Clients))

	var wg sync.WaitGroup
	c := make(chan string)
	wg.Add(len(tlo.Clients))
	go func() {
		wg.Wait()
		close(c)
	}()
	for _, clientCfg := range tlo.Clients {
		go processClient(c, multi, &wg, clientCfg)
	}

	for val := range c {
		*files = append(*files, val)
	}
}

func processClient(c chan string, multi *pterm.MultiPrinter, wg *sync.WaitGroup, clientCfg tlo_config.ClientT) {
	defer (*wg).Done()
	client := Connect(clientCfg.Login, clientCfg.Pass, clientCfg.Host, clientCfg.Port, clientCfg.SSL)

	s := "all" //"downloading" //"completed"
	tl, _ := client.Torrents(qbt.TorrentsOptions{Filter: &s})

	pb1, _ := pterm.DefaultProgressbar.WithTotal(len(tl)).WithWriter((*multi).NewWriter()).Start("Files from: " + clientCfg.Name)
	for i, t := range tl {
		if (i+1)%1000 == 0 {
			pb1.Add(1000)
		}

		files, err := client.TorrentFiles(t.Hash)
		if err != nil {
			log.Panicln(err)
		}
		for _, file := range files {
			//*files = append(*files, t.SavePath+string(os.PathSeparator)+file.Name)
			c <- t.SavePath + string(os.PathSeparator) + replacer(file.Name)
		}
	}
	pb1.Add(pb1.Total - pb1.Current)
	pb1.Stop()
}

func compare(localFiles []string, torrentFiles []string, multi *pterm.MultiPrinter) (diffList []string) {

	pterm.DefaultBasicText.WithWriter((*multi).NewWriter()).Printf("Count files in clients: %d\nCount local files: %d", len(torrentFiles), len(localFiles))

	lfMap := make(map[string]struct{})

	spinnerInfo, _ := pterm.DefaultSpinner.WithWriter((*multi).NewWriter()).Start("Compare files lists")

	for _, val := range torrentFiles {
		lfMap[strings.ToLower(val)] = struct{}{}
	}

	for _, val := range localFiles {
		if _, ok := lfMap[strings.ToLower(val)]; !ok {
			diffList = append(diffList, val)
		}
	}
	spinnerInfo.Success()
	return
}
