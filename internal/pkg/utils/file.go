package utils

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/airenas/go-app/pkg/goapp"
)

// WriteFile write file to disk
func WriteFile(name string, data []byte) error {
	goapp.Log.Info().Str("name", name).Msg("Save")
	f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// FileExists check if file exists
func FileExists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

var supportedExt = map[string]struct{}{".wav": {}, ".mp3": {}, ".mp4": {}, ".m4a": {}, ".ogg": {}, ".wma": {}, ".webm": {}}

// SupportAudioExt checks if audio ext is supported
func SupportAudioExt(ext string) bool {
	_, ok := supportedExt[ext]
	return ok
}

// ParamTrue - returns true if string param indicates true value
func ParamTrue(prm string) bool {
	return strings.ToLower(prm) == "true" || prm == "1"
}

// MakeFileName prepares a file for file keeper
func MakeFileName(ID, fileName string) string {
	res, _ := MakeValidateFileName(ID, fileName)
	return res
}

// MakeValidateFileName prepares/sanitizes a file for file keeper and validates
func MakeValidateFileName(ID, fileName string) (string, error) {
	return url.JoinPath(ID, sanitizeName(toLowerExt(filepath.Base(fileName))))
}

func toLowerExt(f string) string {
	ext := filepath.Ext(f)
	return f[:len(f)-len(ext)] + strings.ToLower(ext)
}

func sanitizeName(s string) string {
	return strings.ReplaceAll(s, " ", "_")
}
