// Writed by yijian on 2020/03/19
// 轻量级支持按大小滚动和多进程的日志
//
// 最简使用步骤：
// var mylog simlog.SimLogger
// 1）定义日志对象
// mylog.Init()
// 2）使用之前必须先初始化（如果存在log目录，则日志文件放在log目录，否则放在程序文件的同目录）
// mylog.Infof("%s\n", "hello world")
// 3）记录INFO级别日志
//
// 注意：
// 1）默认不记录源代码文件名和行号，因为记录源代码文件和行号可能影响性能，如需要可调用EnableLogCaller(true)打开
// 2）日志时间记录到微秒
// 3）如果有再包装，则应设置好skip值，设置方法参考skip成员的说明，不然记录的源代码文件名和行号将不正确
package simlog

import (
    "fmt"
    "os"
    "path/filepath"
    "runtime"
    "syscall"
    "time"
)

// 日志级别（Log Level）
type LogLevel int
const (
    LL_FATAL LogLevel = 0
    LL_ERROR LogLevel = 1
    LL_WARNING LogLevel = 2
    LL_NOTICE LogLevel = 3
    LL_INFO LogLevel = 4
    LL_DEBUG LogLevel = 5
    LL_DETAIL LogLevel = 6 // 比DEBUG更详细的级别
    LL_TRACE LogLevel = 7 // 跟踪日志，独立的日志级别
    LL_RAW LogLevel = 8 // 裸日志，独立的日志级别
)

// 使用之前，应先调用SimLogger的Init进行初始化
type SimLogger struct {
    logCaller bool // 是否记录调用者（在go中取源代码文件名和行号有性能影响，所以默认是关闭的）
    printScreen bool // 是否屏幕打印（默认为false）
    enableTraceLog bool // 是否开启跟踪日志，不能通过logLevel来控制跟踪日志
    enableLineFeed bool // 是否自动换行（默认为false，即不自动换行）
    logLevel LogLevel // 日志级别（默认为LL_INFO）
    logFileSize int64 // 单个日志文件大小（参考值，实际可能超出，默认为100M）
    logNumBackups int // 日志文件备份数（默认为包括当前的在内的共10个）
    logFilename string // 日志文件名（不包含目录部分）
    logDir string // 日志目录（不包含文件名部分）
    skip int // 源代码所在跳（默认为3，但如果有对SimLogger包装调用，则包装一层应当设置为4，包装两层设置为5，依次类推）
}

// Init应在SimLogger所有其它成员被调用之前调用
func (this* SimLogger) Init() bool {
    this.logCaller = false
    this.printScreen = false
    this.enableTraceLog = false
    this.enableLineFeed = false
    this.skip = 3

    this.logLevel = LL_INFO
    this.logFilename = GetLogFilename("")
    this.logDir = GetLogDir()
    this.logFileSize = 1024 * 1024 * 100
    this.logNumBackups = 10
    return true
}

// 设置日志目录（不包含文件名部分）
func (this* SimLogger) SetLogDir(logDir string) {
    this.logDir = logDir
}

// 设置日志文件名（不包含目录部分）
func (this* SimLogger) SetLogFilename(logFilename string) {
    this.logFilename = logFilename
}

func (this* SimLogger) SetSkip(skip int) {
    this.skip = skip
}

func (this* SimLogger) EnableLogCaller(enabled bool) {
    this.logCaller = enabled
}

func (this* SimLogger) EnablePrintScreen(enabled bool) {
    this.printScreen = enabled
}

func (this* SimLogger) EnableTraceLog(enabled bool) {
    this.enableTraceLog = enabled
}

func (this* SimLogger) EnableLineFeed(enabled bool) {
    this.enableLineFeed = enabled
}

// 设置日志级别
func (this* SimLogger) SetLogLevel(logLevel LogLevel) {
    this.logLevel = logLevel
}

// 设置单个日志文件字节数（参考值）
func (this* SimLogger) SetLogFileSize(logFileSize int64) {
    this.logFileSize = logFileSize
}

// 设置日志文件备份数
func (this* SimLogger) SetNumBackups(logNumBackups int) {
    this.logNumBackups = logNumBackups
}

func (this* SimLogger) Rawf(format string, a ...interface{}) {
    //　TODO
}

func (this* SimLogger) Tracef(format string, a ...interface{}) {
    if this.enableTraceLog {
        file, line := this.getCaller()
        this.logf(LL_TRACE, file, line, format, a...)
    }
}

func (this* SimLogger) Detailf(format string, a ...interface{}) {
    if this.logLevel >= LL_DETAIL {
        file, line := this.getCaller()
        this.logf(LL_DETAIL, file, line, format, a...)
    }
}

func (this* SimLogger) Debugf(format string, a ...interface{}) {
    if this.logLevel >= LL_DEBUG {
        file, line := this.getCaller()
        this.logf(LL_DEBUG, file, line, format, a...)
    }
}

func (this* SimLogger) Infof(format string, a ...interface{}) {
    if this.logLevel >= LL_INFO {
        file, line := this.getCaller()
        this.logf(LL_INFO, file, line, format, a...)
    }
}

func (this* SimLogger) Noticef(format string, a ...interface{}) {
    if this.logLevel >= LL_NOTICE {
        file, line := this.getCaller()
        this.logf(LL_NOTICE, file, line, format, a...)
    }
}

func (this* SimLogger) Warningf(format string, a ...interface{}) {
    if this.logLevel >= LL_WARNING {
        file, line := this.getCaller()
        this.logf(LL_WARNING, file, line, format, a...)
    }
}

func (this* SimLogger) Errorf(format string, a ...interface{}) {
    if this.logLevel >= LL_ERROR {
        file, line := this.getCaller()
        this.logf(LL_ERROR, file, line, format, a...)
    }
}

func (this* SimLogger) Fatalf(format string, a ...interface{}) {
    if this.logLevel >= LL_FATAL {
        file, line := this.getCaller()
        this.logf(LL_FATAL, file, line, format, a...)
        os.Exit(1) // 致使错误
    }
}

// 返回调用者所在源代码文件名和行号
func (this* SimLogger) getCaller() (string, int) {
    var file string
    var line int = 0
    if this.logCaller {
        _, file, line, _ = runtime.Caller(this.skip)
    }
    return file, line
}

// 组装日志行头
func (this* SimLogger) formatLogLineHeader(logLevel LogLevel, file string, line int) string {
    now := time.Now()

    if file != "" && line > 0 {
        return fmt.Sprintf("[%s][%d-%d-%d %d:%d:%d/%d][%s:%d]",
            GetLogLevelName(logLevel),
            now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), now.Nanosecond()/1000,
            filepath.Base(file), line)
    } else {
        return fmt.Sprintf("[%s][%d-%d-%d %d:%d:%d/%d]",
            GetLogLevelName(logLevel),
            now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), now.Nanosecond()/1000)
    }
}

// logLevel: 日志级别
// file: 源代码文件名（不包含目录部分）
// line: 源代码行号
func (this* SimLogger) logf(logLevel LogLevel, file string, line int, format string, a ...interface{}) {
    cur_filepath := fmt.Sprintf("%s/%s", this.logDir, this.logFilename)
    logLineHeader := this.formatLogLineHeader(logLevel, file, line)
    // 日志打屏
    if this.printScreen {
        if this.enableLineFeed {
            fmt.Printf(logLineHeader+format+"\n", a ...)
        } else {
            fmt.Printf(logLineHeader+format, a ...)
        }
    }

    // 日志写文件
    // 0644 -> rw-r--r--
    f, err := os.OpenFile(cur_filepath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
    if err == nil {
        defer f.Close()

        fi, err := f.Stat()
        if err == nil {
            logFileSize := fi.Size()

            if this.enableLineFeed {
                f.WriteString(fmt.Sprintf(logLineHeader+format+"\n", a ...))
            } else {
                f.WriteString(fmt.Sprintf(logLineHeader+format, a ...))
            }
            if logFileSize >= this.logFileSize {
                this.rotateLog(cur_filepath, f)
            }
        }
    }
}

func (this* SimLogger) rotateLog(cur_filepath string, f *os.File) {
    // 进入滚动逻辑
    // 先加文件锁，进一步判断
    // syscall.LOCK_EX: 排他锁
    err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX) // syscall.LOCK_NB
    if err == nil {
        defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

        logFileSize, err := GetFileSize(cur_filepath)
        if err == nil && logFileSize >= this.logFileSize {
            // 正式进入滚动逻辑
            for i := this.logNumBackups - 1; i > 0; i-- {
                new_filepath := fmt.Sprintf("%s/%s.%d", this.logDir, this.logFilename, i)
                old_filepath := fmt.Sprintf("%s/%s.%d", this.logDir, this.logFilename, i-1)
                os.Rename(old_filepath, new_filepath)
            }
            if this.logNumBackups > 0 {
                new_filepath := fmt.Sprintf("%s/%s.%d", this.logDir, this.logFilename, 1)
                os.Rename(cur_filepath, new_filepath)
            } else {
                os.Remove(cur_filepath)
            }
        }
    }
}

/**
 * 以下为全局函数区
 */

// 取得指定文件的文件大小
func GetFileSize(filepath string) (int64, error) {
    fi, e := os.Stat(filepath)
    if e != nil {
        return int64(-1), e
    } else {
        return fi.Size(), nil
    }
}

// 根据日志级别得到对应级别名
func GetLogLevelName(logLevel LogLevel) string {
    logLevelNameArray := [...]string{
        "FATAL",
        "ERROR",
        "WARNING",
        "NOTICE",
        "INFO",
        "DEBUG",
        "DETAIL",
        "TRACE",
        "RAW" }
    return logLevelNameArray[int(logLevel)]
}

// 自动取日志目录，
// 如果取不到日志目录，则将日志文件放到程序同目录
func GetLogDir() string {
    binDir := filepath.Dir(os.Args[0])
    logDir := fmt.Sprintf("%s/../log", binDir)
    fi, err := os.Stat(logDir)
    if err != nil {
        return binDir
    } else {
        if fi.IsDir() {
            return logDir
        } else {
            return binDir
        }
    }
}

// 自动取日志文件名，后缀总是为“.log”，
// 可指定子后缀（FILENAME.SUBSUFFIX.log），如果不指定则无子后缀（FILENAME.log）
func GetLogFilename(subSuffix string) string {
    logFilename, err := os.Executable()
    if err == nil {
        if subSuffix == "" {
            return fmt.Sprintf("%s.log", filepath.Base(logFilename))
        } else {
            return fmt.Sprintf("%s.%s.log", subSuffix, filepath.Base(logFilename))
        }
    } else {
        if subSuffix == "" {
            return fmt.Sprintf("%s.log", filepath.Base(os.Args[0]))
        } else {
            return fmt.Sprintf("%s.%s.log", subSuffix, filepath.Base(os.Args[0]))
        }
    }
}
