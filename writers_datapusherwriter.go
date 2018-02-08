// Copyright (c) 2012 - Cloud Instruments Co., Ltd.
//
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this
//    list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright notice,
//    this list of conditions and the following disclaimer in the documentation
//    and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE FOR
// ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
// (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
// LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
// ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
// SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package seelog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// consoleWriter is used to write to console
type DataPusherWriter struct {
	AccessKey string
	Desc      string
	Event     string
}

// Creates a new console writer. Returns error, if the console writer couldn't be created.
func NewDataPusherWriter(accessKey string, desc string, event string) *DataPusherWriter {
	newWriter := new(DataPusherWriter)
	newWriter.AccessKey = accessKey
	newWriter.Desc = desc
	newWriter.Event = event
	return newWriter
}

// Create folder and file on WriteLog/Write first call
func (datapusher *DataPusherWriter) Write(bytes []byte) (int, error) {
	go datapusher.postDate(string(bytes))
	return 0, nil
}

func (datapusher *DataPusherWriter) postDate(content string) error {
	postdata := make(map[string]interface{})
	postdata["desc"] = datapusher.Desc
	postdata["content"] = content

	bytesData, err := json.Marshal(postdata)
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: 3 * time.Second,
	}
	invokeUrl := fmt.Sprintf("http://123.207.72.235:3000/v1/project/%s/events/%s", datapusher.AccessKey, datapusher.Event)
	req, err := http.NewRequest("POST", invokeUrl, bytes.NewBuffer(bytesData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (datapusher *DataPusherWriter) String() string {
	return "DashPusher writer"
}
