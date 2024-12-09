package requests

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"net/url"
	"os"

	"net/http"

	"github.com/gospider007/gtls"
	"github.com/gospider007/ja3"
	"github.com/gospider007/re"
	"github.com/gospider007/tools"
	"github.com/gospider007/websocket"
	utls "github.com/refraction-networking/utls"
)

type contextKey string

const gospiderContextKey contextKey = "GospiderContextKey"

var errFatal = errors.New("ErrFatal")

var ErrUseLastResponse = http.ErrUseLastResponse

type reqCtxData struct {
	isWs                  bool
	h3                    bool
	forceHttp1            bool
	maxRedirect           int
	proxy                 *url.URL
	proxys                []*url.URL
	disProxy              bool
	orderHeaders          []string
	responseHeaderTimeout time.Duration
	tlsHandshakeTimeout   time.Duration
	tlsConfig             *tls.Config
	utlsConfig            *utls.Config

	requestCallBack func(context.Context, *http.Request, *http.Response) error

	h2Ja3Spec ja3.H2Ja3Spec
	ja3Spec   ja3.Ja3Spec

	dialTimeout time.Duration
	keepAlive   time.Duration
	localAddr   *net.TCPAddr  //network card ip
	addrType    gtls.AddrType //first ip type
	dns         *net.UDPAddr
	isNewConn   bool
	logger      func(Log)
	requestId   string
}

func NewReqCtxData(ctx context.Context, option *RequestOption) (*reqCtxData, error) {
	//init ctxData
	ctxData := new(reqCtxData)
	ctxData.requestId = tools.NaoId()
	ctxData.logger = option.Logger
	ctxData.h3 = option.H3
	ctxData.tlsConfig = option.TlsConfig
	ctxData.utlsConfig = option.UtlsConfig
	ctxData.ja3Spec = option.Ja3Spec
	ctxData.h2Ja3Spec = option.H2Ja3Spec
	ctxData.forceHttp1 = option.ForceHttp1
	ctxData.maxRedirect = option.MaxRedirect
	ctxData.requestCallBack = option.RequestCallBack
	ctxData.responseHeaderTimeout = option.ResponseHeaderTimeout
	ctxData.addrType = option.AddrType
	ctxData.dialTimeout = option.DialTimeout
	ctxData.keepAlive = option.KeepAlive
	ctxData.localAddr = option.LocalAddr
	ctxData.dns = option.Dns
	ctxData.disProxy = option.DisProxy
	ctxData.tlsHandshakeTimeout = option.TlsHandshakeTimeout
	//init scheme
	if option.Url != nil {
		if option.Url.Scheme == "ws" {
			ctxData.isWs = true
			option.Url.Scheme = "http"
		} else if option.Url.Scheme == "wss" {
			ctxData.isWs = true
			option.Url.Scheme = "https"
		}
	}
	//init tls timeout
	if option.TlsHandshakeTimeout == 0 {
		ctxData.tlsHandshakeTimeout = time.Second * 15
	}
	//init proxy
	if option.Proxy != "" {
		tempProxy, err := gtls.VerifyProxy(option.Proxy)
		if err != nil {
			return nil, tools.WrapError(errFatal, errors.New("tempRequest init proxy error"), err)
		}
		ctxData.proxy = tempProxy
	}
	if l := len(option.Proxys); l > 0 {
		ctxData.proxys = make([]*url.URL, l)
		for i, proxy := range option.Proxys {
			tempProxy, err := gtls.VerifyProxy(proxy)
			if err != nil {
				return ctxData, tools.WrapError(errFatal, errors.New("tempRequest init proxy error"), err)
			}
			ctxData.proxys[i] = tempProxy
		}
	}
	return ctxData, nil
}
func CreateReqCtx(ctx context.Context, ctxData *reqCtxData) context.Context {
	return context.WithValue(ctx, gospiderContextKey, ctxData)
}
func GetReqCtxData(ctx context.Context) *reqCtxData {
	return ctx.Value(gospiderContextKey).(*reqCtxData)
}

// sends a GET request and returns the response.
func Get(ctx context.Context, href string, options ...RequestOption) (resp *Response, err error) {
	return defaultClient.Request(ctx, http.MethodGet, href, options...)
}

// sends a Head request and returns the response.
func Head(ctx context.Context, href string, options ...RequestOption) (resp *Response, err error) {
	return defaultClient.Request(ctx, http.MethodHead, href, options...)
}

// sends a Post request and returns the response.
func Post(ctx context.Context, href string, options ...RequestOption) (resp *Response, err error) {
	return defaultClient.Request(ctx, http.MethodPost, href, options...)
}

// sends a Put request and returns the response.
func Put(ctx context.Context, href string, options ...RequestOption) (resp *Response, err error) {
	return defaultClient.Request(ctx, http.MethodPut, href, options...)
}

// sends a Patch request and returns the response.
func Patch(ctx context.Context, href string, options ...RequestOption) (resp *Response, err error) {
	return defaultClient.Request(ctx, http.MethodPatch, href, options...)
}

// sends a Delete request and returns the response.
func Delete(ctx context.Context, href string, options ...RequestOption) (resp *Response, err error) {
	return defaultClient.Request(ctx, http.MethodDelete, href, options...)
}

// sends a Connect request and returns the response.
func Connect(ctx context.Context, href string, options ...RequestOption) (resp *Response, err error) {
	return defaultClient.Request(ctx, http.MethodConnect, href, options...)
}

// sends a Options request and returns the response.
func Options(ctx context.Context, href string, options ...RequestOption) (resp *Response, err error) {
	return defaultClient.Request(ctx, http.MethodOptions, href, options...)
}

// sends a Trace request and returns the response.
func Trace(ctx context.Context, href string, options ...RequestOption) (resp *Response, err error) {
	return defaultClient.Request(ctx, http.MethodTrace, href, options...)
}

// Define a function named Request that takes in four parameters:
func Request(ctx context.Context, method string, href string, options ...RequestOption) (resp *Response, err error) {
	return defaultClient.Request(ctx, method, href, options...)
}

// sends a Get request and returns the response.
func (obj *Client) Get(ctx context.Context, href string, options ...RequestOption) (*Response, error) {
	return obj.Request(ctx, http.MethodGet, href, options...)
}

// sends a Head request and returns the response.
func (obj *Client) Head(ctx context.Context, href string, options ...RequestOption) (*Response, error) {
	return obj.Request(ctx, http.MethodHead, href, options...)
}

// sends a Post request and returns the response.
func (obj *Client) Post(ctx context.Context, href string, options ...RequestOption) (*Response, error) {
	return obj.Request(ctx, http.MethodPost, href, options...)
}

// sends a Put request and returns the response.
func (obj *Client) Put(ctx context.Context, href string, options ...RequestOption) (*Response, error) {
	return obj.Request(ctx, http.MethodPut, href, options...)
}

// sends a Patch request and returns the response.
func (obj *Client) Patch(ctx context.Context, href string, options ...RequestOption) (*Response, error) {
	return obj.Request(ctx, http.MethodPatch, href, options...)
}

// sends a Delete request and returns the response.
func (obj *Client) Delete(ctx context.Context, href string, options ...RequestOption) (*Response, error) {
	return obj.Request(ctx, http.MethodDelete, href, options...)
}

// sends a Connect request and returns the response.
func (obj *Client) Connect(ctx context.Context, href string, options ...RequestOption) (*Response, error) {
	return obj.Request(ctx, http.MethodConnect, href, options...)
}

// sends a Options request and returns the response.
func (obj *Client) Options(ctx context.Context, href string, options ...RequestOption) (*Response, error) {
	return obj.Request(ctx, http.MethodOptions, href, options...)
}

// sends a Trace request and returns the response.
func (obj *Client) Trace(ctx context.Context, href string, options ...RequestOption) (*Response, error) {
	return obj.Request(ctx, http.MethodTrace, href, options...)
}

// Define a function named Request that takes in four parameters:
func (obj *Client) Request(ctx context.Context, method string, href string, options ...RequestOption) (response *Response, err error) {
	if obj.closed {
		return nil, errors.New("client is closed")
	}
	if ctx == nil {
		ctx = obj.ctx
	}
	var rawOption RequestOption
	if len(options) > 0 {
		rawOption = options[0]
	}
	optionBak := obj.newRequestOption(rawOption)
	if optionBak.Method == "" {
		optionBak.Method = method
	}
	uhref := optionBak.Url
	if uhref == nil {
		if uhref, err = url.Parse(href); err != nil {
			err = tools.WrapError(err, "url parse error")
			return
		}
	}
	for maxRetries := 0; maxRetries <= optionBak.MaxRetries; maxRetries++ {
		option := optionBak
		option.Url = cloneUrl(uhref)
		option.client = obj
		response, err = obj.request(ctx, &option)
		if err == nil || errors.Is(err, errFatal) || option.once {
			return
		}
	}
	return
}
func (obj *Client) request(ctx context.Context, option *RequestOption) (response *Response, err error) {
	response = new(Response)
	defer func() {
		//read body
		if err == nil && !response.IsWebSocket() && !response.IsSse() && !response.IsStream() {
			err = response.ReadBody()
		}
		//result callback
		if err == nil && option.ResultCallBack != nil {
			err = option.ResultCallBack(ctx, option, response)
		}
		if err != nil { //err callback, must close body
			response.CloseBody()
			if option.ErrCallBack != nil {
				if err2 := option.ErrCallBack(ctx, option, response, err); err2 != nil {
					err = tools.WrapError(errFatal, err2)
				}
			}
		}
	}()
	if option.OptionCallBack != nil {
		if err = option.OptionCallBack(ctx, option); err != nil {
			return
		}
	}
	response.requestOption = option
	//init headers and orderheaders,befor init ctxData
	headers, orderHeaders, err := option.initHeaders()
	if err != nil {
		return response, tools.WrapError(err, errors.New("tempRequest init headers error"), err)
	}
	if headers != nil && option.UserAgent != "" {
		headers.Set("User-Agent", option.UserAgent)
	}
	if orderHeaders == nil {
		orderHeaders = option.OrderHeaders
	}
	//设置 h2 请求头顺序
	if orderHeaders != nil {
		if !option.H2Ja3Spec.IsSet() {
			option.H2Ja3Spec = ja3.DefaultH2Ja3Spec()
			option.H2Ja3Spec.OrderHeaders = orderHeaders
		} else if option.H2Ja3Spec.OrderHeaders == nil {
			option.H2Ja3Spec.OrderHeaders = orderHeaders
		}
	}
	//init ctxData
	response.reqCtxData, err = NewReqCtxData(ctx, option)
	if err != nil {
		return response, tools.WrapError(err, " reqCtxData init error")
	}
	if headers == nil {
		headers = defaultHeaders()
	}
	//设置 h1 请求头顺序
	if orderHeaders != nil {
		response.reqCtxData.orderHeaders = orderHeaders
	} else {
		response.reqCtxData.orderHeaders = ja3.DefaultOrderHeaders()
	}
	//init ctx,cnl
	if option.Timeout > 0 { //超时
		response.ctx, response.cnl = context.WithTimeout(CreateReqCtx(ctx, response.reqCtxData), option.Timeout)
	} else {
		response.ctx, response.cnl = context.WithCancel(CreateReqCtx(ctx, response.reqCtxData))
	}
	//init url
	href, err := option.initParams()
	if err != nil {
		err = tools.WrapError(err, "url init error")
		return
	}
	if href.User != nil {
		headers.Set("Authorization", "Basic "+tools.Base64Encode(href.User.String()))
	}
	//init body
	body, err := option.initBody(response.ctx)
	if err != nil {
		return response, tools.WrapError(err, errors.New("tempRequest init body error"), err)
	}
	//create request
	reqs, err := NewRequestWithContext(response.ctx, option.Method, href, body)
	if err != nil {
		return response, tools.WrapError(errFatal, errors.New("tempRequest 构造request失败"), err)
	}
	reqs.Header = headers
	//add Referer
	if reqs.Header.Get("Referer") == "" {
		if option.Referer != "" {
			reqs.Header.Set("Referer", option.Referer)
		} else if reqs.URL.Scheme != "" && reqs.URL.Host != "" {
			reqs.Header.Set("Referer", fmt.Sprintf("%s://%s", reqs.URL.Scheme, reqs.URL.Host))
		}
	}

	//set ContentType
	if option.ContentType != "" && reqs.Header.Get("Content-Type") == "" {
		reqs.Header.Set("Content-Type", option.ContentType)
	}

	//init ws
	if response.reqCtxData.isWs {
		websocket.SetClientHeadersWithOption(reqs.Header, option.WsOption)
	}

	if reqs.URL.Scheme == "file" {
		response.filePath = re.Sub(`^/+`, "", reqs.URL.Path)
		response.content, err = os.ReadFile(response.filePath)
		if err != nil {
			err = tools.WrapError(errFatal, errors.New("read filePath data error"), err)
		}
		return
	}
	//add host
	if option.Host != "" {
		reqs.Host = option.Host
	} else if reqs.Header.Get("Host") != "" {
		reqs.Host = reqs.Header.Get("Host")
	} else {
		reqs.Host = reqs.URL.Host
	}

	//init cookies
	cookies, err := option.initCookies()
	if err != nil {
		return response, tools.WrapError(err, errors.New("tempRequest init cookies error"), err)
	}
	if cookies != nil {
		addCookie(reqs, cookies)
	}
	//send req
	response.response, err = obj.do(reqs, option)
	if err != nil && err != ErrUseLastResponse {
		err = tools.WrapError(err, "client do error")
		return
	}
	if response.response == nil {
		err = errors.New("response is nil")
		return
	}
	if response.Body() != nil {
		response.rawConn = response.Body().(*readWriteCloser)
	}
	if !response.requestOption.DisUnZip {
		response.requestOption.DisUnZip = response.response.Uncompressed
	}
	if response.response.StatusCode == 101 {
		response.webSocket = websocket.NewClientConn(response.rawConn.Conn(), websocket.GetResponseHeaderOption(response.response.Header))
	} else if strings.Contains(response.response.Header.Get("Content-Type"), "text/event-stream") {
		response.sse = newSse(response.Body())
	} else if !response.requestOption.DisUnZip {
		var unCompressionBody io.ReadCloser
		unCompressionBody, err = tools.CompressionDecode(response.Body(), response.ContentEncoding())
		if err != nil {
			if err != io.ErrUnexpectedEOF && err != io.EOF {
				return
			}
		}
		if unCompressionBody != nil {
			response.response.Body = unCompressionBody
		}
	}
	return
}
