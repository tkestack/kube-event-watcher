/*
 * Tencent is pleased to support the open source community by making TKEStack
 * available.
 *
 * Copyright (C) 2012-2020 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */
package main

import (
	"encoding/json"
	"time"

	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"bufio"
	"io/ioutil"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
)

// Event fetched from api-server
type Event struct {
	ResourceVersion string `json:"resourceVersion"`
	Type            string `json:"type"`
	LastTimestamp   string `json:"lastTimestamp"`
	FirstTimestamp  string `json:"firstTimestamp"`
	Count           int32  `json:"count"`
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	Kind            string `json:"kind"`
	Reason          string `json:"reason"`
	Source          string `json:"source"`
	Message         string `json:"message"`
}

const (
	EVENT_DIR = "/data/"
)

var (
	fileTempIndex int
)

// AnalysisEvent will parse the useful field for composing the json marshaled Event
func AnalysisEvent(obj interface{}) string {
	var content Event
	event := obj.(*v1.Event)
	content.ResourceVersion = event.ResourceVersion
	content.Type = event.Type
	content.LastTimestamp = event.LastTimestamp.Format("2006-01-02 15:04:05")
	content.FirstTimestamp = event.FirstTimestamp.Format("2006-01-02 15:04:05")
	content.Count = event.Count
	content.Name = event.InvolvedObject.Name
	content.Namespace = event.InvolvedObject.Namespace
	content.Kind = event.InvolvedObject.Kind
	content.Reason = event.Reason
	content.Source = event.Source.Component + event.Source.Host
	content.Message = event.Message
	res, _ := json.Marshal(event)

	return string(res[:])
}

// CheckFileIsExist checks if the files to store events exists
func CheckFileIsExist(filename string) bool {
	_, err := os.Stat(filename)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

// GetLastEventIndex will return the last event index in the index file.
// it will return zero
func GetLastEventIndex() (int, error) {
	indexFileDir := EVENT_DIR + "index"
	if CheckFileIsExist(indexFileDir) {
		indexFile, err := os.OpenFile(indexFileDir, os.O_RDONLY, 0666)
		if err != nil {
			return 0, err
		}
		r, err := ioutil.ReadAll(indexFile)
		if err != nil {
			return 0, err
		}
		if string(r[:]) != "" {
			fileIndex, err := strconv.Atoi(string(r[:]))
			if err != nil {
				return 0, err
			}
			return fileIndex, nil
		}
	}
	return 0, nil
}

// Wrfiles to store events existsiteEventFileFlag will write the index of the log file in to a special file.
func WriteEventFileFlag(index string) error {
	// hand the latest index
	objIndex, _ := strconv.Atoi(index)
	if objIndex < fileTempIndex {
		return nil
	}

	indexFileDir := EVENT_DIR + "index"
	// initialize the index file
	indexFile, err := os.OpenFile(indexFileDir, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		fileTempIndex = objIndex
		return err
	}
	wFlag := bufio.NewWriterSize(indexFile, 1024)
	_, err = wFlag.Write([]byte(index))
	if err != nil {
		fileTempIndex = objIndex
		return err
	}
	err = wFlag.Flush()
	if err != nil {
		fileTempIndex = objIndex
		return err
	}
	// in the final, update the file index
	fileTempIndex = objIndex
	return nil
}

// WriteEventFile writes events to local file
func WriteEventFile(obj interface{}) error {
	if obj != nil {
		// parse the obj
		content := AnalysisEvent(obj)
		// make the directory
		dir := EVENT_DIR + "log/"
		logFileDir := dir + time.Now().Format("2006-01-02") + ".log"
		if !CheckFileIsExist(dir) {
			os.MkdirAll(dir, 0666)
		}
		logfile, err := os.OpenFile(logFileDir, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return err
		}
		defer logfile.Close()
		// write the event log file
		wLog := bufio.NewWriter(logfile)
		_, err = wLog.Write([]byte(content + "\n"))
		if err != nil {
			return err
		}
		err = wLog.Flush()

		// record the resource version
		err = WriteEventFileFlag(obj.(*v1.Event).ResourceVersion)
		if err != nil {
			return err
		}
	}
	return nil
}

func watchEvent(clientset *kubernetes.Clientset) {
	watchlist := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "events", metaV1.NamespaceAll,
		fields.Everything())

	fileTempIndex, err := GetLastEventIndex()
	if err != nil {
		logrus.Error("Failed to get last event index")
	}

	_, controller := cache.NewInformer(
		watchlist,
		&v1.Event{},
		time.Second*0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				objIndex, _ := strconv.Atoi(obj.(*v1.Event).ResourceVersion)
				// judge that it was not a repeated event
				if objIndex > fileTempIndex {
					err = WriteEventFile(obj)
					if err != nil {
						logrus.Error("Failed to write event file")
					}
				}
				logrus.Info("Add", objIndex)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				objIndex, _ := strconv.Atoi(newObj.(*v1.Event).ResourceVersion)
				// judge that it was not a repeated event
				if objIndex > fileTempIndex {
					err := WriteEventFile(newObj)
					if err != nil {
						logrus.Error("Failed to write event file")
					}
				}
				logrus.Info("UpdateFunc", objIndex)
			},
		},
	)
	go controller.Run(wait.NeverStop)
}

func main() {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	// Create a k8s client
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	go watchEvent(clientSet)

	// Block forever
	select {}
}
