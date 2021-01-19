// Writed by yijian on 2020/12/25
package main

import (
    "fmt"
    "os"
    "sync"
)
import (
    "github.com/eyjian/simlog"
)

func main() {
    var wg sync.WaitGroup
    var simlogger simlog.SimLogger

    simlogger.SetSubPrefix("PREFIX")
    simlogger.SetSubSuffix("SUFFIX")
    if !simlogger.Init() {
        fmt.Printf("Init simlog failed\n")
        os.Exit(1)
    }

    simlogger.Infof("Info level\n") // 1
    simlogger.SetTag("TEST")
    simlogger.Infof("Tag is set to TEST\n") // 2

    simlogger.EnableLogCaller(true)
    simlogger.Infof("Log caller enabled\n") // 3

    simlogger.EnableLineFeed(true)
    simlogger.Infof("Linefeed enabled") // 4

    simlogger.EnableRawLog(true, false)
    simlogger.Raw("raw log") // 5

    simlogger.EnableRawLog(true, true)
    simlogger.Raw("raw log with time") // 6

    for i:=0; i<2021; i++ {
        wg.Add(1)
        go func (i int) {
            simlogger.Infof("%d", i)
            wg.Done()
        }(i)
    } // 2027
    wg.Wait()

    simlogger.SetLogObserver(logObserver)
    simlogger.Infof("Exit now") // 2028
    simlogger.Close()
}

func logObserver(logLevel simlog.LogLevel, logHeader string, logBody string) {
    fmt.Printf("[OBSERVED][%s]%s%s\n", simlog.GetLogLevelName(logLevel), logHeader, logBody)
}
