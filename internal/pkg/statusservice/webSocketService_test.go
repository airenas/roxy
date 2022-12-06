package statusservice

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/airenas/roxy/internal/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var (
	wsService *WSConnKeeper
)

func initWSTest(t *testing.T) {
	wsService = NewWSConnKeeper()
}

func createTestConn(t *testing.T, id string, closeChan <-chan struct{}) *mockWSConn {
	t.Helper()
	connWSMock := &mockWSConn{}
	connWSMock.On("WriteJSON", mock.Anything).Return(nil)
	connWSMock.On("ReadMessage").Return(1, []byte(id), nil).Once()
	connWSMock.On("ReadMessage").Return(1, []byte(id), fmt.Errorf("err")).Run(func(args mock.Arguments) {
		<-closeChan
	})
	connWSMock.On("Close").Return(nil)
	return connWSMock
}

func Test_HandleConnection(t *testing.T) {
	initWSTest(t)
	closeCtx, cf := context.WithCancel(test.Ctx(t))
	go func() {
		err := wsService.HandleConnection(createTestConn(t, "1", closeCtx.Done()))
		assert.Nil(t, err)
	}()
	testHas(t, "1", 1)
	cf()
}

func testHas(t *testing.T, s string, i int) {
	t.Helper()
	ctx := test.Ctx(t)
	for {
		cn, ok := wsService.GetConnections(s)
		if ok == (i > 0) && len(cn) == i {
			break
		}
		select {
		case <-ctx.Done():
			require.Failf(t, "timeouted, not found connection %s", s)
		case <-time.After(time.Millisecond * 100):
		}
	}
}

func Test_HandleConnection_Several(t *testing.T) {
	initWSTest(t)
	closeCtx, cf := context.WithCancel(test.Ctx(t))
	for i := 0; i < 10; i++ {
		go func() {
			err := wsService.HandleConnection(createTestConn(t, fmt.Sprintf("%d", 1), closeCtx.Done()))
			assert.Nil(t, err)
		}()
	}
	testHas(t, "1", 10)
	cf()
}

func Test_HandleConnection_SeveralDifferent(t *testing.T) {
	initWSTest(t)
	closeCtx, cf := context.WithCancel(test.Ctx(t))
	for i := 0; i < 10; i++ {
		_i := i
		go func() {
			err := wsService.HandleConnection(createTestConn(t, fmt.Sprintf("%d", _i), closeCtx.Done()))
			assert.Nil(t, err)
		}()
	}
	for i := 0; i < 10; i++ {
		testHas(t, "1", 1)
	}
	cf()
}

func Test_HandleConnection_Cleans(t *testing.T) {
	initWSTest(t)
	closeCtx, cf := context.WithCancel(test.Ctx(t))
	for i := 0; i < 10; i++ {
		_i := i
		go func() {
			err := wsService.HandleConnection(createTestConn(t, fmt.Sprintf("%d", _i), closeCtx.Done()))
			assert.Nil(t, err)
		}()
	}
	testHas(t, "1", 1)
	cf()
	for i := 0; i < 10; i++ {
		testHas(t, "1", 0)
	}
}
