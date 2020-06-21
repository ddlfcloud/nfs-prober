// MIT License

// Copyright (c) 2020 ddlfcloud

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	mrand "math/rand"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

var (
	usePrometheus      = flag.Bool("use_prometheus", true, "create a web endpoint and log timeseries metrics to that endpoint, default true")
	localMountLocation = flag.String("local_mount_dir", "/etc/prober-nfs", "directory to mount nfs targets")
	readAndWrite       = flag.Bool("rw_test_files", false, "read and write test files and log results, default false")
	numOfTestFiles     = flag.Int("num_of_files", 1, "number of test files to read and write, default 1")
	testFileSize       = flag.Int("file_size_bytes", 200, "test file size in bytes, default 200")
	targets            = flag.String("targets", "", "comma seperated list of targets in format ip:/mountPoint")
	interval           = flag.String("interval", "60s", "interval between probes, default 60s")
	timeout            = flag.String("timeout", "250ms", "timeout of probe operation, default 250ms")
	webPort            = flag.Int("port", 8080, "port for web server to listen on")
	version            = flag.String("nfs_version", "nfs", "nfs version to use, eg nfs, nfs3")
)

type nfs struct {
	address    string
	mountPoint string
	log        *logrus.Logger
}

var (
	status = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nfs_status",
		Help: "current mount status of an NFS target",
	}, []string{"address", "mount_point"})
	mountAttempts = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "nfs_mount_attempts",
		Help: "attempts made to connect to an NFS target",
	}, []string{"address", "mount_point", "success"})
	readAttempts = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "nfs_read_attempts",
		Help: "attempts to read a file from a target NFS instance",
	}, []string{"address", "mount_point", "testFile", "success"})
	writeAttempts = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "nfs_write_attempts",
		Help: "attempts to write a file to a target NFS instance",
	}, []string{"address", "mount_point", "testFile", "success"})
	ready = false
)

func (n *nfs) unmount(ctx context.Context) {
	syscall.Unmount(fmt.Sprintf("%s/%s", *localMountLocation, n.address), 0)
}

func (n *nfs) mount(ctx context.Context) error {
	// Ensure NFS is unmounted before starting
	n.unmount(ctx)
	// Start Time to be used for all duration logs
	startTime := time.Now()
	// Use syscall to mount the NFS directory
	err := syscall.Mount(fmt.Sprintf(":%s", n.mountPoint), fmt.Sprintf("%s/%s", *localMountLocation, n.address), *version, 0, fmt.Sprintf("nolock,addr=%s", n.address))
	duration := time.Since(startTime).Seconds()
	if err != nil {
		n.log.WithFields(logrus.Fields{"success": false, "address": n.address, "mountPoint": n.mountPoint, "err": err, "duration": duration}).Warn("could not mount")
		if *usePrometheus {
			status.WithLabelValues(n.address, n.mountPoint).Set(0)
			mountAttempts.WithLabelValues(n.address, n.mountPoint, "false").Observe(duration)
		}
		n.unmount(ctx)
		return err
	}
	n.log.WithFields(logrus.Fields{"success": true, "address": n.address, "mountPoint": n.mountPoint, "duration": duration}).Info("mount successful")
	if *usePrometheus {
		status.WithLabelValues(n.address, n.mountPoint).Set(1)
		mountAttempts.WithLabelValues(n.address, n.mountPoint, "true").Observe(duration)
	}
	return nil
}

func (n *nfs) readTestFiles(ctx context.Context) {
	for i := 0; i < *numOfTestFiles; i++ {
		testFileLocation := fmt.Sprintf("%s/%s/%d", *localMountLocation, n.address, i)
		startTime := time.Now()
		b, err := ioutil.ReadFile(testFileLocation)
		duration := time.Since(startTime).Seconds()
		if err != nil {
			n.log.WithFields(logrus.Fields{"success": false, "address": n.address, "mountPoint": n.mountPoint, "err": err, "duration": duration, "file": testFileLocation}).Warn("could not read test file")
			if *usePrometheus {
				readAttempts.WithLabelValues(n.address, n.mountPoint, testFileLocation, "false").Observe(duration)
			}
			continue
		}
		if len(b) != *testFileSize {
			n.log.WithFields(logrus.Fields{"success": false, "address": n.address, "mountPoint": n.mountPoint, "err": fmt.Sprintf("got %d bytes from file, but expected %d bytes", len(b), *testFileSize), "duration": duration, "file": testFileLocation}).Warn("could not read test file")
			if *usePrometheus {
				readAttempts.WithLabelValues(n.address, n.mountPoint, testFileLocation, "false").Observe(duration)
			}
		}
		n.log.WithFields(logrus.Fields{"success": true, "address": n.address, "mountPoint": n.mountPoint, "duration": duration, "file": testFileLocation}).Info("read test file")
		if *usePrometheus {
			readAttempts.WithLabelValues(n.address, n.mountPoint, testFileLocation, "true").Observe(duration)
		}
	}
}

func (n *nfs) writeTestFiles(ctx context.Context) {
	for i := 0; i < *numOfTestFiles; i++ {
		testFileLocation := fmt.Sprintf("%s/%s/%d", *localMountLocation, n.address, i)
		b := make([]byte, *testFileSize)
		_, err := rand.Read(b)
		if err != nil {
			n.log.WithFields(logrus.Fields{"success": false, "address": n.address, "mountPoint": n.mountPoint, "err": err, "file": testFileLocation}).Warn("could create test file")
			continue
		}
		startTime := time.Now()
		err = ioutil.WriteFile(testFileLocation, b, 0644)
		duration := time.Since(startTime).Seconds()
		if err != nil {
			n.log.WithFields(logrus.Fields{"success": false, "address": n.address, "mountPoint": n.mountPoint, "err": err, "duration": duration, "file": testFileLocation}).Warn("could not write test file")
			if *usePrometheus {
				writeAttempts.WithLabelValues(n.address, n.mountPoint, testFileLocation, "false").Observe(duration)
			}
			continue
		}
		// make sure the number of bytes read matches the file size
		if len(b) != *testFileSize {
			n.log.WithFields(logrus.Fields{"success": false, "address": n.address, "mountPoint": n.mountPoint, "err": fmt.Sprintf("got %d bytes from file, but expected %d bytes", len(b), *testFileSize), "duration": duration, "file": testFileLocation}).Warn("could not read test file")
			if *usePrometheus {
				writeAttempts.WithLabelValues(n.address, n.mountPoint, testFileLocation, "false").Observe(duration)
			}
		}
		n.log.WithFields(logrus.Fields{"success": true, "address": n.address, "mountPoint": n.mountPoint, "duration": duration, "file": testFileLocation}).Info("write test file")
		if *usePrometheus {
			writeAttempts.WithLabelValues(n.address, n.mountPoint, testFileLocation, "true").Observe(duration)
		}
	}
}

func (n *nfs) test(ctx context.Context) {
	intervalDur, err := time.ParseDuration(*interval)
	if err != nil {
		n.log.Fatal(err)
	}
	timeoutDur, err := time.ParseDuration(*timeout)
	if err != nil {
		n.log.Fatal(err)
	}
	ticker := time.NewTicker(intervalDur)
	done := make(chan bool)
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			ctxWithTimeout, cancel := context.WithTimeout(ctx, timeoutDur)
			defer cancel()
			err := n.mount(ctxWithTimeout)
			if err != nil {
				continue
			}
			if *readAndWrite {
				n.writeTestFiles(ctx)
				n.readTestFiles(ctx)
			}
		}
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if ready {
		w.WriteHeader(200)
		return
	}
	w.WriteHeader(500)
	return
}

func main() {
	flag.Parse()
	newLog := logrus.New()
	newLog.Out = os.Stdout
	if *targets == "" {
		log.Print("please specify targets")
	}
	// Max of 5 files allowed.
	if *numOfTestFiles > 5 {
		*numOfTestFiles = 5
	}
	ctx := context.Background()

	// Get list of NFS targets from cmd line arguments
	listOfTargets := strings.Split(*targets, ",")
	go func() {
		// Loop through all targets and start probes concurrently
		for n, target := range listOfTargets {
			s := strings.Split(target, ":")
			if len(s) < 2 {
				log.Printf("target %s was not in correct format", target)
				os.Exit(1)
			}
			// Only mount to the "prober" directory. This should not be changed.
			mountPoint := fmt.Sprintf("%s/%s", s[1], "prober")
			address := s[0]
			// Make all local directories needed for mounting
			os.MkdirAll(fmt.Sprintf("%s/%s", *localMountLocation, address), os.ModePerm)
			newTarget := &nfs{
				address:    address,
				mountPoint: mountPoint,
				log:        newLog,
			}
			// Wait a random amount of time from 0 - 30s so targets don't start at the same time
			mrand.Seed(time.Now().UnixNano() + int64(n))
			time.Sleep(time.Duration(mrand.Intn(30)) * time.Second)
			go newTarget.test(ctx)
		}
	}()
	ready = true
	http.HandleFunc("/health", healthHandler)
	if *usePrometheus {
		http.Handle("/metrics", promhttp.Handler())
	}
	logrus.Info(fmt.Sprintf("starting HTTP endpoint on :%d", *webPort))
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *webPort), nil))
}
