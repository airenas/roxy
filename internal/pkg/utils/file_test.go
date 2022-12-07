package utils

import "testing"

func TestMakeValidateFileName(t *testing.T) {
	type args struct {
		ID       string
		fileName string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{name: "OK", args: args{ID: "2", fileName: "olia.txt"}, want: "2/olia.txt", wantErr: false},
		{name: "OK", args: args{ID: "2", fileName: "./olia.txt"}, want: "2/olia.txt", wantErr: false},
		{name: "OK", args: args{ID: "2", fileName: "./../olia.txt"}, want: "2/olia.txt", wantErr: false},
		{name: "OK UPPER", args: args{ID: "2", fileName: "./1/Olia.TXT"}, want: "2/Olia.txt", wantErr: false},
		{name: "OK change space", args: args{ID: "2", fileName: "./1/Olia one.TXT"}, want: "2/Olia_one.txt", wantErr: false},
		{name: "No start", args: args{ID: "", fileName: "./1/Olia one.TXT"}, want: "Olia_one.txt", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MakeValidateFileName(tt.args.ID, tt.args.fileName)
			if (err != nil) != tt.wantErr {
				t.Errorf("MakeValidateFileName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("MakeValidateFileName() = %v, want %v", got, tt.want)
			}
		})
	}
}
