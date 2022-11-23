package utils

import (
	"os"
	"strings"

	"github.com/airenas/go-app/pkg/goapp"
)

//WriteFile write file to disk
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

//FileExists check if file exists
func FileExists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

//SupportAudioExt checks if audio ext is supported
func SupportAudioExt(ext string) bool {
	return ext == ".wav" || ext == ".mp3" || ext == ".mp4" || ext == ".m4a"
}

// ParamTrue - returns true if string param indicates true value
func ParamTrue(prm string) bool {
	return strings.ToLower(prm) == "true" || prm == "1"
}
