package requests

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"reflect"

	"github.com/gospider007/gson"
	"github.com/gospider007/tools"
	"golang.org/x/exp/maps"
)

const (
	readType = iota
	mapType
)

type orderMap struct {
	data  map[string]any
	order []string
}

func NewOrderMap() *orderMap {
	return &orderMap{
		data:  make(map[string]any),
		order: []string{},
	}
}

func (obj *orderMap) Set(key string, val any) {
	obj.Del(key)
	obj.data[key] = val
	obj.order = append(obj.order, key)
}
func (obj *orderMap) Del(key string) {
	delete(obj.data, key)
	obj.order = tools.DelSliceVals(obj.order, key)
}
func (obj *orderMap) parseHeaders() (map[string][]string, []string) {
	head := make(http.Header)
	for _, kk := range obj.order {
		if vvs, ok := obj.data[kk].([]any); ok {
			for _, vv := range vvs {
				head.Add(kk, fmt.Sprint(vv))
			}
		} else {
			head.Add(kk, fmt.Sprint(obj.data[kk]))
		}
	}
	return head, obj.order
}

func formWrite(writer *multipart.Writer, key string, val any) (err error) {
	switch value := val.(type) {
	case File:
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeQuotes(key), escapeQuotes(value.FileName)))
		if value.ContentType == "" {
			switch content := value.Content.(type) {
			case []byte:
				h.Set("Content-Type", http.DetectContentType(content))
			case string:
				h.Set("Content-Type", http.DetectContentType(tools.StringToBytes(content)))
			case io.Reader:
				h.Set("Content-Type", "application/octet-stream")
			default:
				con, err := gson.Encode(content)
				if err != nil {
					return err
				}
				h.Set("Content-Type", http.DetectContentType(con))
			}
		} else {
			h.Set("Content-Type", value.ContentType)
		}
		var wp io.Writer
		if wp, err = writer.CreatePart(h); err != nil {
			return
		}
		switch content := value.Content.(type) {
		case []byte:
			_, err = wp.Write(content)
		case string:
			_, err = wp.Write(tools.StringToBytes(content))
		case io.Reader:
			_, err = io.Copy(wp, content)
		default:
			con, err := gson.Encode(content)
			if err != nil {
				return err
			}
			_, err = wp.Write(con)
		}
	case []byte:
		err = writer.WriteField(key, tools.BytesToString(value))
	case string:
		err = writer.WriteField(key, value)
	default:
		con, err := gson.Encode(val)
		if err != nil {
			return err
		}
		err = writer.WriteField(key, tools.BytesToString(con))
	}
	return
}
func (obj *orderMap) parseForm(ctx context.Context) (io.Reader, string, bool, error) {
	if len(obj.order) == 0 || len(obj.data) == 0 {
		return nil, "", false, nil
	}
	if obj.isformPip() {
		pr, pw := pipe(ctx)
		writer := multipart.NewWriter(pw)
		go func() {
			err := obj.formWriteMain(writer)
			if err == nil {
				err = io.EOF
			}
			pr.Close(err)
			pw.Close(err)
		}()
		return pr, writer.FormDataContentType(), true, nil
	}
	body := bytes.NewBuffer(nil)
	writer := multipart.NewWriter(body)
	err := obj.formWriteMain(writer)
	if err != nil {
		return nil, writer.FormDataContentType(), false, err
	}
	return body, writer.FormDataContentType(), false, err
}
func (obj *orderMap) isformPip() bool {
	if len(obj.order) == 0 || len(obj.data) == 0 {
		return false
	}
	for _, key := range obj.order {
		if vals, ok := obj.data[key].([]any); ok {
			for _, val := range vals {
				if file, ok := val.(File); ok {
					if _, ok := file.Content.(io.Reader); ok {
						return true
					}
				}
			}
		} else {
			if file, ok := obj.data[key].(File); ok {
				if _, ok := file.Content.(io.Reader); ok {
					return true
				}
			}
		}
	}
	return false
}
func (obj *orderMap) formWriteMain(writer *multipart.Writer) (err error) {
	for _, key := range obj.order {
		if vals, ok := obj.data[key].([]any); ok {
			for _, val := range vals {
				if err = formWrite(writer, key, val); err != nil {
					return
				}
			}
		} else {
			if err = formWrite(writer, key, obj.data[key]); err != nil {
				return
			}
		}
	}
	return writer.Close()
}

func paramsWrite(buf *bytes.Buffer, key string, val any) {
	if buf.Len() > 0 {
		buf.WriteByte('&')
	}
	buf.WriteString(url.QueryEscape(key))
	buf.WriteByte('=')
	buf.WriteString(url.QueryEscape(fmt.Sprint(val)))
}
func (obj *orderMap) parseParams() *bytes.Buffer {
	buf := bytes.NewBuffer(nil)
	for _, k := range obj.order {
		if vals, ok := obj.data[k].([]any); ok {
			for _, v := range vals {
				paramsWrite(buf, k, v)
			}
		} else {
			paramsWrite(buf, k, obj.data[k])
		}
	}
	return buf
}
func (obj *orderMap) parseData() *bytes.Reader {
	val := obj.parseParams().Bytes()
	if val == nil {
		return nil
	}
	return bytes.NewReader(val)
}
func (obj *orderMap) MarshalJSON() ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	err := buf.WriteByte('{')
	if err != nil {
		return nil, err
	}
	for i, k := range obj.order {
		if i > 0 {
			if err = buf.WriteByte(','); err != nil {
				return nil, err
			}
		}
		key, err := gson.Encode(k)
		if err != nil {
			return nil, err
		}
		if _, err = buf.Write(key); err != nil {
			return nil, err
		}
		if err = buf.WriteByte(':'); err != nil {
			return nil, err
		}
		val, err := gson.Encode(obj.data[k])
		if err != nil {
			return nil, err
		}
		if _, err = buf.Write(val); err != nil {
			return nil, err
		}
	}
	if err = buf.WriteByte('}'); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func any2Map(val any) map[string]any {
	if reflect.TypeOf(val).Kind() != reflect.Map {
		return nil
	}
	mapValue := reflect.ValueOf(val)
	result := make(map[string]any)
	for _, key := range mapValue.MapKeys() {
		valueData := mapValue.MapIndex(key).Interface()
		sliceValue := reflect.ValueOf(valueData)
		if sliceValue.Kind() == reflect.Slice {
			valueData2 := []any{}
			for i := 0; i < sliceValue.Len(); i++ {
				valueData2 = append(valueData2, sliceValue.Index(i).Interface())
			}
			result[fmt.Sprint(key.Interface())] = valueData2
		} else {
			result[fmt.Sprint(key.Interface())] = valueData
		}
	}
	return result
}

func (obj *RequestOption) newBody(val any, valType int) (reader io.Reader, parseOrderMap *orderMap, orderKey []string, err error) {
	if reader, ok := val.(io.Reader); ok {
		obj.once = true
		return reader, nil, nil, nil
	}
	if valType == readType {
		switch value := val.(type) {
		case string:
			return bytes.NewReader(tools.StringToBytes(value)), nil, nil, nil
		case []byte:
			return bytes.NewReader(value), nil, nil, nil
		default:
			enData, err := gson.Encode(value)
			if err != nil {
				return nil, nil, nil, err
			}
			return bytes.NewReader(enData), nil, nil, nil
		}
	}
	if mapData := any2Map(val); mapData != nil {
		val = mapData
	}
mapL:
	switch value := val.(type) {
	case *gson.Client:
		if !value.IsObject() {
			return nil, nil, nil, errors.New("body-type error")
		}
		orderMap := NewOrderMap()
		for kk, vv := range value.Map() {
			if vv.IsArray() {
				valData := make([]any, len(vv.Array()))
				for i, v := range vv.Array() {
					valData[i] = v.Value()
				}
				orderMap.Set(kk, valData)
			} else {
				orderMap.Set(kk, vv.Value())
			}
		}
		return nil, orderMap, nil, nil
	case *orderMap:
		if mapData := any2Map(value.data); mapData != nil {
			value.data = mapData
		}
		return nil, value, nil, nil
	case map[string]any:
		orderMap := NewOrderMap()
		orderMap.data = value
		orderMap.order = maps.Keys(value)
		return nil, orderMap, nil, nil
	}
	if val, err = gson.Decode(val); err != nil {
		return nil, nil, nil, err
	}
	goto mapL
}
