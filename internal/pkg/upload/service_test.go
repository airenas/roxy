package upload

// var (
// 	saverMock  *mocks.MockFileSaver
// 	rSaverMock *mocks.MockRequestSaver
// 	senderMock *mocks.MockMsgSender
// 	tData      *Data
// 	tEcho      *echo.Echo
// 	tResp      *httptest.ResponseRecorder
// )

// func initTest(t *testing.T) {
// 	mocks.AttachMockToTest(t)
// 	saverMock = mocks.NewMockFileSaver()
// 	rSaverMock = mocks.NewMockRequestSaver()
// 	senderMock = mocks.NewMockMsgSender()
// 	tData = &Data{}
// 	tData.Saver = saverMock
// 	tData.ReqSaver = rSaverMock
// 	tData.MsgSender = senderMock
// 	tData.Configurator, _ = NewTTSConfigurator("mp3", "astra", []string{"vyt"})
// 	tEcho = initRoutes(tData)
// 	tResp = httptest.NewRecorder()
// }

// func TestWrongPath(t *testing.T) {
// 	initTest(t)
// 	req := httptest.NewRequest(http.MethodGet, "/invalid", nil)
// 	testCode(t, req, 404)
// }

// func TestWrongMethod(t *testing.T) {
// 	initTest(t)
// 	req := httptest.NewRequest(http.MethodGet, "/upload", nil)
// 	testCode(t, req, 405)
// }

// func Test_Returns(t *testing.T) {
// 	initTest(t)
// 	req := newTestRequest("file", "file.txt", "olia", nil)
// 	resp := testCode(t, req, 200)
// 	bytes, _ := ioutil.ReadAll(resp.Body)
// 	assert.Contains(t, string(bytes), `"id":"`)
// }

// func Test_400(t *testing.T) {
// 	type args struct {
// 		filep, file string
// 		params      [][2]string
// 	}
// 	tests := []struct {
// 		name     string
// 		args     args
// 		wantCode int
// 	}{
// 		{name: "OK", args: args{file: "file.txt", filep: "file"}, wantCode: http.StatusOK},
// 		{name: "File", args: args{file: "file.txt", filep: "file1"}, wantCode: http.StatusBadRequest},
// 		{name: "FileName", args: args{file: "file", filep: "file"}, wantCode: http.StatusBadRequest},
// 		{name: "FileName", args: args{file: "file.wav", filep: "file"}, wantCode: http.StatusBadRequest},
// 		{name: "Voice", args: args{file: "file.txt", filep: "file", params: [][2]string{{"voice", "astra"}}},
// 			wantCode: http.StatusOK},
// 		{name: "Voice", args: args{file: "file.txt", filep: "file", params: [][2]string{{"voice", "astra1"}}},
// 			wantCode: http.StatusBadRequest},
// 		{name: "Voice", args: args{file: "file.txt", filep: "file", params: [][2]string{{"outputFormat", "wav"}}},
// 			wantCode: http.StatusBadRequest},
// 		{name: "Voice", args: args{file: "file.txt", filep: "file", params: [][2]string{{"speed", "aa"}}},
// 			wantCode: http.StatusBadRequest},
// 		{name: "Voice", args: args{file: "file.txt", filep: "file", params: [][2]string{{"outputFormat", "mp3"}}},
// 			wantCode: http.StatusOK},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			initTest(t)
// 			req := newTestRequest(tt.args.filep, tt.args.file, "olia", tt.args.params)
// 			testCode(t, req, tt.wantCode)
// 		})
// 	}
// }

// func Test_Fails_Saver(t *testing.T) {
// 	initTest(t)
// 	req := newTestRequest("file", "file.txt", "olia", nil)
// 	pegomock.When(saverMock.Save(pegomock.AnyString(), matchers.AnyIoReader())).ThenReturn(errors.New("err"))

// 	testCode(t, req, http.StatusInternalServerError)
// }

// func Test_Fails_ReqSaver(t *testing.T) {
// 	initTest(t)
// 	req := newTestRequest("file", "file.txt", "olia", nil)
// 	pegomock.When(rSaverMock.Save(matchers.AnyPtrToPersistenceReqData())).ThenReturn(errors.New("err"))

// 	testCode(t, req, http.StatusInternalServerError)
// }

// func Test_Fails_MsgSender(t *testing.T) {
// 	initTest(t)
// 	req := newTestRequest("file", "file.txt", "olia", nil)
// 	pegomock.When(senderMock.Send(matchers.AnyMessagesMessage(), pegomock.AnyString(), pegomock.AnyString())).ThenReturn(errors.New("err"))

// 	testCode(t, req, http.StatusInternalServerError)
// }

// func Test_Live(t *testing.T) {
// 	initTest(t)
// 	req := httptest.NewRequest(http.MethodGet, "/live", nil)
// 	testCode(t, req, 200)
// }

// func testCode(t *testing.T, req *http.Request, code int) *httptest.ResponseRecorder {
// 	tEcho.ServeHTTP(tResp, req)
// 	assert.Equal(t, code, tResp.Code)
// 	return tResp
// }

// func Test_validate(t *testing.T) {
// 	type args struct {
// 		data *Data
// 	}
// 	tests := []struct {
// 		name    string
// 		args    args
// 		wantErr bool
// 	}{
// 		{name: "OK", args: args{data: &Data{Saver: mocks.NewMockFileSaver(),
// 			Configurator: &TTSConfigutaror{}, ReqSaver: mocks.NewMockRequestSaver(),
// 			MsgSender: mocks.NewMockMsgSender()}}, wantErr: false},
// 		{name: "Fail Saver", args: args{data: &Data{
// 			Configurator: &TTSConfigutaror{}, ReqSaver: mocks.NewMockRequestSaver(),
// 			MsgSender: mocks.NewMockMsgSender()}}, wantErr: true},
// 		{name: "Fail Configurator", args: args{data: &Data{Saver: mocks.NewMockFileSaver(),
// 			ReqSaver:  mocks.NewMockRequestSaver(),
// 			MsgSender: mocks.NewMockMsgSender()}}, wantErr: true},
// 		{name: "Fail ReqSaver", args: args{data: &Data{Saver: mocks.NewMockFileSaver(),
// 			Configurator: &TTSConfigutaror{},
// 			MsgSender:    mocks.NewMockMsgSender()}}, wantErr: true},
// 		{name: "Fail Sender", args: args{data: &Data{Saver: mocks.NewMockFileSaver(),
// 			Configurator: &TTSConfigutaror{}, ReqSaver: mocks.NewMockRequestSaver()}}, wantErr: true},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			if err := validate(tt.args.data); (err != nil) != tt.wantErr {
// 				t.Errorf("StartWebServer() error = %v, wantErr %v", err, tt.wantErr)
// 			}
// 		})
// 	}
// }

// func newTestRequest(filep, file, bodyText string, params [][2]string) *http.Request {
// 	body := &bytes.Buffer{}
// 	writer := multipart.NewWriter(body)
// 	if file != "" {
// 		part, _ := writer.CreateFormFile(filep, file)
// 		_, _ = io.Copy(part, strings.NewReader(bodyText))
// 	}
// 	for _, p := range params {
// 		writer.WriteField(p[0], p[1])
// 	}
// 	writer.Close()
// 	req := httptest.NewRequest("POST", "/upload", body)
// 	req.Header.Set("Content-Type", writer.FormDataContentType())
// 	req.Header.Set(requestIDHEader, "m:testRequestID")
// 	return req
// }

// func Test_extractRequestID(t *testing.T) {
// 	req := newTestRequest("file", "file.txt", "olia", nil)
// 	assert.Equal(t, "m:testRequestID", extractRequestID(req.Header))
// }
