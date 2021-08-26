package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/pingcap/tidb/util/logutil"
	"github.com/tidb-incubator/weir/pkg/cc"
	"github.com/tidb-incubator/weir/pkg/config"
	"go.uber.org/zap"
)

var (
	configFilePath = flag.String("config", "conf/weirproxycc.yaml", "weirproxy cc config file path")
)

func main() {
	flag.Parse()
	ccConfigData, err := ioutil.ReadFile(*configFilePath)
	if err != nil {
		fmt.Printf("read config file error: %v\n", err)
		os.Exit(1)
	}

	ccConfig, err := config.UnmarshalCCConfig(ccConfigData)
	if err != nil {
		fmt.Printf("parse config file error: %v\n", err)
		os.Exit(1)
	}

	s, err := cc.NewServer(ccConfig)
	if err != nil {
		fmt.Printf("create server failed: %v\n", err)
		os.Exit(1)
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
		syscall.SIGPIPE,
		syscall.SIGUSR1,
	)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			sig := <-sc
			if sig == syscall.SIGINT || sig == syscall.SIGTERM || sig == syscall.SIGQUIT {
				logutil.BgLogger().Warn("get os signal, close cc server", zap.String("signal", sig.String()))
				s.Close()
				break
			} else {
				logutil.BgLogger().Warn("ignore os signal", zap.String("signal", sig.String()))
			}
		}
	}()

	s.Run()
	wg.Wait()
}
