package httpsvr

import (
	"net/http"
	"time"
	"bytes"
	"io/ioutil"
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/julienschmidt/httprouter"
	logger "github.com/shengkehua/xlog4go"

)

var defaultResponse = []byte(`{"errno":0,"errmsg":"ok"}`)

// Server ...
type Server struct {
	addr	string
	router	*httprouter.Router
	opt 	*option
	oriSvr	*http.Server
	ac 	*Access
}

func New(addr string, opts ...ServerOption) *Server {
	opt := &option{}
	for _, o := range opts {
		o(opt)
	}
	if addr == "" {
		addr = "127.0.0.1:10024"
	}
	s := &Server{
		addr: 	addr,
		router: httprouter.New(),
		opt:	opt,
	}
	if s.opt.maxAccess == 0 {
		s.opt.maxAccess = 1024
	}
	s.ac = NewAccessor(s.opt.maxAccess)
	s.ac.Run()
	s.oriSvr = &http.Server{Addr: addr, Handler: s}
	return s
}

// ServeHTTP implement net/http.router
func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.router.ServeHTTP(w, req)
}

func (s *Server) AddRoute(method, path string, ctrl IController){
	var handle httprouter.Handle = func(w http.ResponseWriter, r *http.Request, params httprouter.Params){
		defer func() {
			if err := recover(); err != nil {
				w.Write([]byte("Server is busy."))
				stack := make([]byte, 2048)
				stack = stack[:runtime.Stack(stack, false)]
				f := "PANIC: %s\n%s"
				logger.Error(f, err, stack)
			}
		}()

		nt := time.Now()
		// 打印输入请求
		if s.opt.dumpAccess {
			body, _ := ioutil.ReadAll(r.Body)
			r.Body = ioutil.NopCloser(bytes.NewBuffer(body))
			logger.Info("request_uri=%s||client_ip=%s||request_body=%s",
				r.URL,
				GetClientAddr(r),
				string(body))
		}
		s.ac.InControl()
		// 解析输入参数
		idl := ctrl.GenIdl()
		body, _ := ioutil.ReadAll(r.Body)
		err := json.Unmarshal(body, idl)
		if err != nil {
			w.WriteHeader(http.StatusOK)
			w.Write(getErrMsg(err))
			return
		}

		do := func(r *http.Request, w http.ResponseWriter) {
			var data []byte
			resp := ctrl.Do(idl)
			if resp == nil {
				data = defaultResponse
			}
			data, _ = json.Marshal(resp)
			et := time.Now().Sub(nt)
			logger.Info("request_uri=%s||response=%s||proc_time=%s",
				r.URL, string(data), et.String())
			w.WriteHeader(200)
			w.Write(data)
		}

		do(r, w)
		s.ac.OutControl()
	}
	s.router.Handle(method, path, handle)
}

func (s *Server) Serve() error{
	fmt.Printf("Serving %s", s.addr)
	return s.oriSvr.ListenAndServe()
}