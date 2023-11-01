package main

import (
	"Engineer/utils"
	"archive/tar"
	"bufio"
	"compress/gzip"
	"errors"
	jsoniter "github.com/json-iterator/go"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var Signal chan bool

func main() {

	token := utils.Conf.GetString("Token")

	Signal = make(chan bool)

	http.HandleFunc("/", SendUpdate)

	go func() {
		for {
			select {
			case <-Signal:
				log.Println("update started")
				err := Deploy(token)
				if err != nil {
					log.Println(err)
				}
			}
		}
	}()

	http.ListenAndServe(utils.Conf.GetString("ListenAddr"), nil)
}

func SendUpdate(w http.ResponseWriter, r *http.Request) {
	if r.RequestURI == "/?Secret="+utils.Conf.GetString("Secret") {
		Signal <- true
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}
}

func FetchInfo(token string, c *http.Client) (*utils.Latest, error) {
	url := utils.Conf.GetString("Repo")

	reqest, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	reqest.Header.Add("Authorization", "Bearer "+token)

	resp, err := c.Do(reqest)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var latest []utils.Latest
	err = jsoniter.Unmarshal(body, &latest)
	if err != nil {
		return nil, err
	}

	var max int
	var index int
	for i := range latest {
		if max < latest[i].Id {
			index = i
			max = latest[i].Id
		}
	}

	return &latest[index], nil
}

func Download(token string, latest *utils.Latest, c *http.Client) error {
	if len(latest.Assets) < 1 {
		return errors.New("no assets")
	}

	latest.Assets[0].Url = strings.Replace(latest.Assets[0].Url, "api.github.com", utils.Conf.GetString("ProxyDomain"), -1)

	reqest, err := http.NewRequest("GET", latest.Assets[0].Url, nil)
	if err != nil {
		return err
	}

	reqest.Header.Add("Authorization", "Bearer "+token)
	reqest.Header.Add("Accept", "application/octet-stream")
	resp, err := c.Do(reqest)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	reader := bufio.NewReaderSize(resp.Body, 32*1024)

	RemoveFile("./" + latest.Assets[0].Name)
	file, err := os.Create("./" + latest.Assets[0].Name)
	defer file.Close()
	if err != nil {
		return err
	}
	// 获得文件的writer对象
	writer := bufio.NewWriter(file)

	_, err = io.Copy(writer, reader)
	return err
}

func Deploy(token string) error {
	client := &http.Client{}

	info, err := FetchInfo(token, client)
	if err != nil {
		return err
	}

	err = Download(token, info, client)
	if err != nil {
		return err
	}

	return UnTar(info.Assets[0].Name)
}

func UnTar(tarball string) error {
	target := utils.Conf.GetString("DestDir")

	RemoveFile(target)
	reader, err := os.Open(tarball)
	if err != nil {
		return err
	}

	gr, err := gzip.NewReader(reader)
	if err != nil {
		return err
	}
	defer gr.Close()
	defer reader.Close()
	tarReader := tar.NewReader(gr)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		path := filepath.Join(target, header.Name)
		info := header.FileInfo()
		if info.IsDir() {
			if err = os.MkdirAll(path, info.Mode()); err != nil {
				return err
			}
			continue
		}

		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(file, tarReader)
		if err != nil {
			return err
		}
	}
	return nil
}

func RemoveFile(target string) {
	_ = os.RemoveAll(target)
}
