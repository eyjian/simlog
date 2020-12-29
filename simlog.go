// Writed by yijian on 2020/03/19
// 轻量级支持按大小滚动和多进程的日志
//
// 最简使用步骤：
// 1）定义日志对象
// var mylog simlog.SimLogger
// 2）使用之前必须调用Init初始化（如果存在log目录，则日志文件放在log目录，否则放在程序文件的同目录）
// mylog.Init()
// 3）记录INFO级别日志
// mylog.Infof("%s\n", "hello world")
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
    "strconv"
    "sync/atomic"
    "syscall"
    "time"
)

// 日志级别（Log Level）
type LogLevel int

// 调用函数 GetLogLevelName，可取得对应级别的字符串值
const (
    LL_FATAL LogLevel = 0
    LL_ERROR LogLevel = 1
    LL_WARNING LogLevel = 2
    LL_NOTICE LogLevel = 3
    LL_INFO LogLevel = 4 // 默认日志级别
    LL_DEBUG LogLevel = 5
    LL_DETAIL LogLevel = 6 // 比DEBUG更详细的级别
    LL_TRACE LogLevel = 7 // 跟踪日志，独立的日志级别
    LL_RAW LogLevel = 8 // 裸日志
)

// 使用之前，应先调用SimLogger的Init进行初始化
// logCaller和printScreen等类型使用int32而不是bool，
// 是为方便原子修改值，比如实时安全地调整日志级别。
type SimLogger struct {
    logCaller int32 // 是否记录调用者（在go中取源代码文件名和行号有性能影响，所以默认是关闭的）
    printScreen int32 // 是否屏幕打印（默认为false）
    enableTraceLog int32 // 是否开启跟踪日志，不能通过logLevel来控制跟踪日志
    enableLineFeed int32 // 是否自动换行（默认为false，即不自动换行）
    enableRawLog int32 // 是否允许裸日志
    rawLogWithTime int32 // 裸日志是否带日期时间头
    logLevel int32 // 日志级别（默认为LL_INFO）
    logFileSize int64 // 单个日志文件大小（参考值，实际可能超出，默认为100M）
    logNumBackups int32 // 日志文件备份数（默认为包括当前的在内的共10个）
    logFilename string // 日志文件名（不包含目录部分）
    logDir string // 日志目录（不包含文件名部分）、
    subSuffix string // 日志文件名子后缀：filename.SUBSUFFIX.log，默认为空表示无子后缀
    tag string // 默认为空，如果不为空，则会作为日志头的一部分，比如可为一个 IP 地址，用来标识日志源于哪
    skip int32 // 源代码所在跳（默认为3，但如果有对SimLogger包装调用，则包装一层应当设置为4，包装两层设置为5，依次类推）
    logObserver LogObserver
}

// 日志观察者，通过设置 LogObserver 可截获日志，比如将截获的日志写入到 Kafka 等
type LogObserver func(logLevel LogLevel, logHeader string, logBody string)

// 设置日志文件名子后缀，
// 只在在使用默认的日志文件名进才有效，并且SetSubSuffix必须在Init之前调用才有效
func (this* SimLogger) SetSubSuffix(subSuffix string) {
    this.subSuffix = subSuffix
}

// 注意 SetTag 不是协程安全的，应当在使用之前调用
func (this* SimLogger) SetTag(tag string) {
    this.tag = tag
}

// 需在 Init 之后调用，但应在写日志之前调用，非协程安全
func (this* SimLogger) SetLogObserver(logObserver LogObserver) {
    this.logObserver = logObserver
}

// Init应在SimLogger所有其它成员被调用之前调用，
// SetSubSuffix成员除外，SetSubSuffix只有在Init之前调用才有效。
func (this* SimLogger) Init() bool {
    this.logCaller = 0
    this.printScreen = 0
    this.enableTraceLog = 0
    this.enableLineFeed = 0
    this.enableRawLog = 0
    this.rawLogWithTime = 0
    this.skip = 3

    this.logLevel = int32(LL_INFO)
    this.logFilename = GetLogFilename(this.subSuffix)
    this.logDir = GetLogDir()
    this.logFileSize = 1024 * 1024 * 100
    this.logNumBackups = 10

    this.logObserver = nil
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

// 调用者所在跳，
// 如果直接使用SimLogger的写日志函数，则默认值3即可，
// 否则每包一层skip值就得加一，否则将不能正确显示源代码文件名和行号。
func (this* SimLogger) SetSkip(skip int32) {
    atomic.StoreInt32(&this.skip, skip)
}

func (this* SimLogger) GetSkip() int32 {
    return atomic.LoadInt32(&this.skip)
}

// 是否开启了记录调用者
func (this* SimLogger) EnabledLogCaller() bool {
    return atomic.LoadInt32(&this.logCaller) == 1
}

// enabled为true表示是否记录源代码文件和行号
func (this* SimLogger) EnableLogCaller(enabled bool) {
    if enabled {
        atomic.StoreInt32(&this.logCaller, 1)
    } else {
        atomic.StoreInt32(&this.logCaller, 0)
    }
}

// withTime 如果为 true 则会加上日期时间头
func (this* SimLogger) EnableRawLog(enabled, withTime bool) {
    if enabled {
        atomic.StoreInt32(&this.enableRawLog, 1)
    } else {
        atomic.StoreInt32(&this.enableRawLog, 0)
    }
    if withTime {
        atomic.StoreInt32(&this.rawLogWithTime, 1)
    } else {
        atomic.StoreInt32(&this.rawLogWithTime, 0)
    }
}

// 是否开启了日志打屏
func (this* SimLogger) EnabledPrintScreen() bool {
    return atomic.LoadInt32(&this.printScreen) == 1
}

// enabled为true表示日志打屏
func (this* SimLogger) EnablePrintScreen(enabled bool) {
    if enabled {
        atomic.StoreInt32(&this.printScreen, 1)
    } else {
        atomic.StoreInt32(&this.printScreen, 0)
    }
}

// 是否打开了跟踪日志
func (this* SimLogger) EnabledTraceLog() bool {
    return atomic.LoadInt32(&this.enableTraceLog) == 1
}

// enabled为true表示开启跟踪日志，
// 注意SetLogLevel不能控制跟踪日志的开启。
func (this* SimLogger) EnableTraceLog(enabled bool) {
    if enabled {
        atomic.StoreInt32(&this.enableTraceLog, 1)
    } else {
        atomic.StoreInt32(&this.enableTraceLog, 0)
    }
}

// 是否开启了自动换行
func (this* SimLogger) EnabledLineFeed() bool {
    return atomic.LoadInt32(&this.enableLineFeed) == 1
}

// 是否自动换行，enabled为true表示开启自动换行
func (this* SimLogger) EnableLineFeed(enabled bool) {
    if enabled {
        atomic.StoreInt32(&this.enableLineFeed, 1)
    } else {
        atomic.StoreInt32(&this.enableLineFeed, 0)
    }
}

// 取得当前日志级别
func (this* SimLogger) GetLogLevel() int32 {
    return atomic.LoadInt32(&this.logLevel)
}

// 设置日志级别
func (this* SimLogger) SetLogLevel(logLevel LogLevel) {
    atomic.StoreInt32(&this.logLevel, int32(logLevel))
}

// 取得单个日志文件大小
func (this* SimLogger) GetLogFileSize() int64{
    return atomic.LoadInt64(&this.logFileSize)
}

// 设置单个日志文件字节数（参考值）
func (this* SimLogger) SetLogFileSize(logFileSize int64) {
    atomic.StoreInt64(&this.logFileSize, logFileSize)
}

// 取得日志备份数
func (this* SimLogger) GetNumBackups() int32 {
    return atomic.LoadInt32(&this.logNumBackups)
}

// 设置日志文件备份数
func (this* SimLogger) SetNumBackups(logNumBackups int) {
    atomic.StoreInt32(&this.logNumBackups, int32(logNumBackups))
}

// 写裸日志

func (this* SimLogger) Raw(a ...interface{}) (int, error) {
    return this.log(LL_RAW, "", 0, a ...)
}

func (this* SimLogger) Rawln(a ...interface{}) (int, error) {
    return this.logln(LL_RAW, "", 0, a ...)
}

func (this* SimLogger) Rawf(format string, a ...interface{}) (int, error) {
    return this.logf(LL_RAW, "", 0, format, a ...)
}

// 写跟踪日志（Trace）

func (this* SimLogger) Trace(a ...interface{}) (int, error) {
    return this.SkipTrace(this.skip, a ...)
}

func (this* SimLogger) Traceln(a ...interface{}) (int, error) {
    return this.SkipTraceln(this.skip, a ...)
}

func (this* SimLogger) Tracef(format string, a ...interface{}) (int, error) {
    return this.SkipTracef(this.skip, format, a ...)
}

// 写跟踪日志（SkipTrace）

func (this* SimLogger) IsEnabledTraceLog() bool {
    return atomic.LoadInt32(&this.enableTraceLog) == 1
}

func (this* SimLogger) SkipTrace(skip int32, a ...interface{}) (int, error) {
    if !this.IsEnabledTraceLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.log(LL_TRACE, file, line, a...)
    }
}

func (this* SimLogger) SkipTraceln(skip int32, a ...interface{}) (int, error) {
    if !this.IsEnabledTraceLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.logln(LL_TRACE, file, line, a...)
    }
}

func (this* SimLogger) SkipTracef(skip int32, format string, a ...interface{}) (int, error) {
    if !this.IsEnabledTraceLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.logf(LL_TRACE, file, line, format, a...)
    }
}

// 写详细日志（Detail）

func (this* SimLogger) IsEnabledDetailLog() bool {
    return atomic.LoadInt32(&this.logLevel) >= int32(LL_DETAIL)
}

func (this* SimLogger) Detail(a ...interface{}) (int, error) {
    return this.SkipDetail(this.skip, a ...)
}

func (this* SimLogger) Detailln(a ...interface{}) (int, error) {
    return this.SkipDetailln(this.skip, a ...)
}

func (this* SimLogger) Detailf(format string, a ...interface{}) (int, error) {
    return this.SkipDetailf(this.skip, format, a ...)
}

// 写详细日志（SkipDetail）

func (this* SimLogger) SkipDetail(skip int32, a ...interface{}) (int, error) {
    if !this.IsEnabledDetailLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.log(LL_DETAIL, file, line, a...)
    }
}

func (this* SimLogger) SkipDetailln(skip int32, a ...interface{}) (int, error) {
    if !this.IsEnabledDetailLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.logln(LL_DETAIL, file, line, a...)
    }
}

func (this* SimLogger) SkipDetailf(skip int32, format string, a ...interface{}) (int, error) {
    if !this.IsEnabledDetailLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.logf(LL_DETAIL, file, line, format, a...)
    }
}

// 写调试日志（Debug）

func (this* SimLogger) IsEnabledDebugLog() bool {
    return atomic.LoadInt32(&this.logLevel) >= int32(LL_DEBUG)
}

func (this* SimLogger) Debug(a ...interface{}) (int, error) {
    return this.SkipDebug(this.skip, a ...)
}

func (this* SimLogger) Debugln(a ...interface{}) (int, error) {
    return this.SkipDebugln(this.skip, a ...)
}

func (this* SimLogger) Debugf(format string, a ...interface{}) (int, error) {
    return this.SkipDebugf(this.skip, format, a ...)
}

// 写调试日志（SkipDebug）

func (this* SimLogger) SkipDebug(skip int32, a ...interface{}) (int, error) {
    if !this.IsEnabledDebugLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.log(LL_DEBUG, file, line, a...)
    }
}

func (this* SimLogger) SkipDebugln(skip int32, a ...interface{}) (int, error) {
    if !this.IsEnabledDebugLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.logln(LL_DEBUG, file, line, a...)
    }
}

func (this* SimLogger) SkipDebugf(skip int32, format string, a ...interface{}) (int, error) {
    if !this.IsEnabledDebugLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.logf(LL_DEBUG, file, line, format, a...)
    }
}

// 写信息日志（Info）

func (this* SimLogger) IsEnabledInfoLog() bool {
    return atomic.LoadInt32(&this.logLevel) >= int32(LL_INFO)
}

func (this *SimLogger) Info(a ...interface{}) (int, error) {
    return this.SkipInfo(this.skip, a ...)
}

func (this *SimLogger) Infoln(a ...interface{}) (int, error) {
    return this.SkipInfoln(this.skip, a ...)
}

func (this* SimLogger) Infof(format string, a ...interface{}) (int, error) {
    return this.SkipInfof(this.skip, format, a ...)
}

// 写信息日志（SkipInfo）

func (this *SimLogger) SkipInfo(skip int32, a ...interface{}) (int, error) {
    if !this.IsEnabledInfoLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.log(LL_INFO, file, line, a ...)
    }
}

func (this *SimLogger) SkipInfoln(skip int32, a ...interface{}) (int, error) {
    if !this.IsEnabledInfoLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.logln(LL_INFO, file, line, a ...)
    }
}

func (this* SimLogger) SkipInfof(skip int32, format string, a ...interface{}) (int, error) {
    if !this.IsEnabledInfoLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.logf(LL_INFO, file, line, format, a...)
    }
}

// 写注意日志（Notice）

func (this* SimLogger) IsEnabledNoticeLog() bool {
    return atomic.LoadInt32(&this.logLevel) >= int32(LL_NOTICE)
}

func (this* SimLogger) Notice(a ...interface{}) (int, error) {
    return this.SkipNotice(this.skip, a ...)
}

func (this* SimLogger) Noticeln(a ...interface{}) (int, error) {
    return this.SkipNoticeln(this.skip, a ...)
}

func (this* SimLogger) Noticef(format string, a ...interface{}) (int, error) {
    return this.SkipNoticef(this.skip, format, a ...)
}

// 写注意日志（SkipNotice）

func (this* SimLogger) SkipNotice(skip int32, a ...interface{}) (int, error) {
    if !this.IsEnabledNoticeLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.log(LL_NOTICE, file, line, a...)
    }
}

func (this* SimLogger) SkipNoticeln(skip int32, a ...interface{}) (int, error) {
    if !this.IsEnabledNoticeLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.logln(LL_NOTICE, file, line, a...)
    }
}

func (this* SimLogger) SkipNoticef(skip int32, format string, a ...interface{}) (int, error) {
    if !this.IsEnabledNoticeLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.logf(LL_NOTICE, file, line, format, a...)
    }
}

// 写警示日志（Warning）

func (this* SimLogger) IsEnabledWarningLog() bool {
    return atomic.LoadInt32(&this.logLevel) >= int32(LL_WARNING)
}

func (this *SimLogger) Warning(a ...interface{}) (int, error) {
    return this.SkipWarning(this.skip, a ...)
}

func (this *SimLogger) Warningln(a ...interface{}) (int, error) {
    return this.SkipWarningln(this.skip, a ...)
}

func (this* SimLogger) Warningf(format string, a ...interface{}) (int, error) {
    return this.SkipWarningf(this.skip, format, a ...)
}

// 写警示日志（SkipWarning）

func (this *SimLogger) SkipWarning(skip int32, a ...interface{}) (int, error) {
    if !this.IsEnabledWarningLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.log(LL_WARNING, file, line, a...)
    }
}

func (this *SimLogger) SkipWarningln(skip int32, a ...interface{}) (int, error) {
    if !this.IsEnabledWarningLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.logln(LL_WARNING, file, line, a...)
    }
}

func (this* SimLogger) SkipWarningf(skip int32, format string, a ...interface{}) (int, error) {
    if !this.IsEnabledWarningLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.logf(LL_WARNING, file, line, format, a...)
    }
}

// 写错误日志（Error）

func (this* SimLogger) IsEnabledErrorLog() bool {
    return atomic.LoadInt32(&this.logLevel) >= int32(LL_ERROR)
}

func (this *SimLogger) Error(a ...interface{}) (int, error) {
    return this.SkipError(this.skip, a ...)
}

func (this *SimLogger) Errorln(a ...interface{}) (int, error) {
    return this.SkipErrorln(this.skip, a ...)
}

func (this* SimLogger) Errorf(format string, a ...interface{}) (int, error) {
    return this.SkipErrorf(this.skip, format, a ...)
}

// 写错误日志（SkipError）

func (this *SimLogger) SkipError(skip int32, a ...interface{}) (int, error) {
    if !this.IsEnabledErrorLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.log(LL_ERROR, file, line, a...)
    }
}

func (this *SimLogger) SkipErrorln(skip int32, a ...interface{}) (int, error) {
    if !this.IsEnabledErrorLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.logln(LL_ERROR, file, line, a...)
    }
}

func (this* SimLogger) SkipErrorf(skip int32, format string, a ...interface{}) (int, error) {
    if !this.IsEnabledErrorLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        return this.logf(LL_ERROR, file, line, format, a...)
    }
}

// 写致命错误日志（Fatal），
// 注意在调用后进程会退出。

func (this* SimLogger) IsEnabledFatalLog() bool {
    return atomic.LoadInt32(&this.logLevel) >= int32(LL_FATAL)
}

func (this *SimLogger) Fatal(a ...interface{}) (int, error) {
    return this.SkipFatal(this.skip, a ...)
}

func (this *SimLogger) Fatalln(a ...interface{}) (int, error) {
    return this.SkipFatalln(this.skip, a ...)
}

func (this* SimLogger) Fatalf(format string, a ...interface{}) (int, error) {
    return this.SkipFatalf(this.skip, format, a ...)
}

// 写致命错误日志（SkipFatal）

func (this *SimLogger) SkipFatal(skip int32, a ...interface{}) (int, error) {
    if !this.IsEnabledFatalLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        n, err := this.log(LL_FATAL, file, line, a...)
        os.Exit(1) // 致使错误
        return n, err
    }
}

func (this *SimLogger) SkipFatalln(skip int32, a ...interface{}) (int, error) {
    if !this.IsEnabledFatalLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        n, err := this.logln(LL_FATAL, file, line, a...)
        os.Exit(1) // 致使错误
        return n, err
    }
}

func (this* SimLogger) SkipFatalf(skip int32, format string, a ...interface{}) (int, error) {
    if !this.IsEnabledFatalLog() {
        return 0, nil
    } else {
        file, line := this.getCaller(skip)
        n, err := this.logf(LL_FATAL, file, line, format, a...)
        os.Exit(1) // 致使错误
        return n, err
    }
}

// 返回调用者所在源代码文件名和行号
func (this* SimLogger) getCaller(skip int32) (string, int) {
    var file string
    var line int = 0
    if atomic.LoadInt32(&this.logCaller) == 1 {
        _, file, line, _ = runtime.Caller(int(skip))
    }
    return file, line
}

// 组装日志行头
func (this* SimLogger) formatLogLineHeader(logLevel LogLevel, file string, line int) string {
    if logLevel == LL_RAW {
        enableRawLog := atomic.LoadInt32(&this.enableRawLog)
        if enableRawLog == 1 {
            rawLogWithTime := atomic.LoadInt32(&this.rawLogWithTime)
            if rawLogWithTime == 1 {
                return getLogTime()
            }
        }
        return ""
    } else {
        var tag string
        var fileline string

        if this.tag != "" {
            tag = "[" + this.tag + "]"
        }
        if file != "" && line > 0 {
            fileline = "[" + filepath.Base(file) + ":" + strconv.FormatInt(int64(line), 10) + "]"
        }

        datetime := getLogTime()
        return tag + fileline + datetime
    }
}

// 实际接口 Writer：
// type Writer interface {
//   Write(p []byte) (n int, err error)
// }
func (this* SimLogger) Write(p []byte) (int, error) {
    return this.writeLog(string(p))
}

func (this* SimLogger) writeLog(logLine string) (int, error) {
    // 日志打屏
    if atomic.LoadInt32(&this.printScreen) == 1 {
        fmt.Print(logLine)
    }

    // 写日志文件
    // 日志写文件
    // 0644 -> rw-r--r--
    cur_filepath := fmt.Sprintf("%s/%s", this.logDir, this.logFilename)
    f, err := os.OpenFile(cur_filepath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
    if err != nil {
        return 0, err
    } else {
        defer f.Close()

        fi, err := f.Stat()
        if err != nil {
            return 0, err
        } else {
            logFileSize := fi.Size()
            n, err := f.WriteString(logLine)
            if logFileSize >= this.logFileSize {
                this.rotateLog(cur_filepath, f)
            }
            return n, err
        }
    }
}

func (this* SimLogger) log(logLevel LogLevel, file string, line int, a ...interface{}) (int, error) {
    var logLine string
    logLineHeader := this.formatLogLineHeader(logLevel, file, line)
    logBody := fmt.Sprint(a ...)

    // 构建日志行
    if atomic.LoadInt32(&this.enableLineFeed) == 1 {
        logLine = logLineHeader + logBody + "\n"
    } else {
        logLine = logLineHeader + logBody
    }
    if this.logObserver != nil {
        this.logObserver(logLevel, logLineHeader, logBody)
    }
    return this.writeLog(logLine)
}

func (this* SimLogger) logln(logLevel LogLevel, file string, line int, a ...interface{}) (int, error) {
    var logLine string
    logLineHeader := this.formatLogLineHeader(logLevel, file, line)
    logBody := fmt.Sprint(a ...)

    // 构建日志行
    logLine = logLineHeader + logBody + "\n"
    if this.logObserver != nil {
        this.logObserver(logLevel, logLineHeader, logBody)
    }
    return this.writeLog(logLine)
}

// logLevel: 日志级别
// file: 源代码文件名（不包含目录部分）
// line: 源代码行号
func (this* SimLogger) logf(logLevel LogLevel, file string, line int, format string, a ...interface{}) (int, error) {
    var logLine string
    logLineHeader := this.formatLogLineHeader(logLevel, file, line)
    logBody := fmt.Sprintf(format, a ...)

    // 构建日志行
    if atomic.LoadInt32(&this.enableLineFeed) == 1 {
        logLine = logLineHeader + logBody + "\n"
    } else {
        logLine = logLineHeader + logBody
    }
    if this.logObserver != nil {
        this.logObserver(logLevel, logLineHeader, logBody)
    }
    return this.writeLog(logLine)
}

func (this* SimLogger) rotateLog(cur_filepath string, f *os.File) {
    // 进入滚动逻辑
    // 先加文件锁，进一步判断
    // syscall.LOCK_EX: 排他锁
    err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX) // syscall.LOCK_NB
    if err == nil {
        defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
        logFileSize := atomic.LoadInt64(&this.logFileSize)
        logNumBackups := atomic.LoadInt32(&this.logNumBackups)

        logFileSize, err := GetFileSize(cur_filepath)
        if err == nil && logFileSize >= logFileSize {
            // 正式进入滚动逻辑
            for i := logNumBackups - 1; i > 0; i-- {
                new_filepath := fmt.Sprintf("%s/%s.%d", this.logDir, this.logFilename, i)
                old_filepath := fmt.Sprintf("%s/%s.%d", this.logDir, this.logFilename, i-1)
                os.Rename(old_filepath, new_filepath)
            }
            if logNumBackups > 0 {
                new_filepath := fmt.Sprintf("%s/%s.%d", this.logDir, this.logFilename, 1)
                os.Rename(cur_filepath, new_filepath)
            } else {
                os.Remove(cur_filepath)
            }
        }
    }
}

// 返回记录日志的时间，格式为：YYYY-MM-DD hh:mm:ss uuuuuu
func getLogTime() string {
    now := time.Now()
    return fmt.Sprintf("[%04d-%02d-%02d %02d:%02d:%02d %06d]",
        now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), now.Nanosecond()/1000)
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
            return fmt.Sprintf("%s-%s.log", filepath.Base(logFilename), subSuffix)
        }
    } else {
        if subSuffix == "" {
            return fmt.Sprintf("%s.log", filepath.Base(os.Args[0]))
        } else {
            return fmt.Sprintf("%s-%s.log", filepath.Base(os.Args[0]), subSuffix)
        }
    }
}
