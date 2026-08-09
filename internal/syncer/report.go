package syncer

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func WriteReport(dir string, direction Direction, results []Result) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "importacao-"+time.Now().Format("20060102-150405")+".log")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	fmt.Fprintf(w, "Importador de saves muOS/Knulli\nData: %s\nDireção: %s\nArquivos: %d\n\n", time.Now().Format(time.RFC3339), direction, len(results))
	for _, r := range results {
		status := "OK"
		if r.Err != nil {
			status = "ERRO: " + r.Err.Error()
		}
		fmt.Fprintf(w, "[%s] %s -> %s | ação=%s | bytes=%d\n", status, r.Source, r.Destination, r.Action, r.Bytes)
	}
	return path, nil
}
