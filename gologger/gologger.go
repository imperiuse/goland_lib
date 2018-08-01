package gologger

import (
	"fmt"
	"github.com/imperiuse/golang_lib/colormap"
	"github.com/imperiuse/golang_lib/concat"
	"io"
	"io/ioutil"
	"runtime"
	"time"
)

// Main interface of GoLogger
type Logger interface {
	Log(lvl LogLvl, msg ...interface{})
	LogC(lvl LogLvl, cf ColorFlag, msg ...interface{})

	Info(...interface{})
	Debug(...interface{})
	Warning(...interface{})
	Error(...interface{})
	Fatal(...interface{})

	Test(...interface{})
	Print(...interface{})
	P()
	Other(...interface{})

	LoggerController

	Close()
}

// Interface Logger Controller describes base control settings of logger: color map and I/O mechanism)
// ALL METHOD UNDER MUTEX!
type LoggerController interface {
	SetColorScheme(colormap.CSM)
	SetColorThemeName(string)

	SetDefaultDestinations(io.Writer, DestinationFlag)
	SetNewDestinations(Destinations)

	SetDestinationLvl(lvl LogLvl, sWriters []io.Writer)
	SetDestinationLvlColor(lvl LogLvl, flag ColorFlag, writer io.Writer)

	DisableDestinationLvl(LogLvl)
	DisableDestinationLvlColor(LogLvl, ColorFlag)

	EnableDestinationLvl(LogLvl)
	EnableDestinationLvlColor(LogLvl, ColorFlag)

	SetAndEnableDestinationLvl(LogLvl, []io.Writer)
	SetAndEnableDestinationLvlColor(LogLvl, ColorFlag, io.Writer)
}

// Constructor Logger
func NewLogger(defaultOutput io.Writer, destFlag DestinationFlag, n, callDepth, settingFlags int, delimiter string, csm colormap.CSM) Logger {

	i := 0
	l := logger{
		make(chan logMsg, n),
		settingFlags,
		delimiter,
		callDepth,
		0,
		func() int { i++; return i },
		make(Destinations, len(defaultDestinations)),
		make(LogMap, len(defaultDestinations)),
		csm,
	}

	l.SetDefaultDestinations(defaultOutput, destFlag)
	go l.writeChanGoroutine()

	return &l
}

// These flags define which text to prefix to each log entry generated by the Logger.
// Bits are or'ed together to control what's printed.
// For example, flags Ldate | Ltime (or LstdFlags) produce,
//	2009/01/23 01:23:23 message
// while flags Ldate | Ltime | Lmicroseconds | Llongfile produce,
//	2009/01/23 01:23:23.123123 /a/b/c/d.go:23: message
const (
	Ldate         = 1 << iota // the date in the local time zone: 2009/01/23
	Ltime                     // the time in the local time zone: 01:23:23
	Lmicroseconds             // microsecond resolution: 01:23:23.123123.  assumes Ltime.
	Llongfile                 // full file name and line number: /a/b/c/d.go:23
	Lshortfile                // final file name element and line number: d.go:23. overrides Llongfile
	LUTC                      // if Ldate or Ltime is set, use UTC rather than the local time zone

	LNoStackTrace // No Run runtime.Caller()  for getting Stack Trace info

	LstdFlags = Ldate | Ltime // initial values for the standard logger
)

type LogLvl int

const (
	INFO LogLvl = iota
	DEBUG
	WARNING
	ERROR
	FATAL
	TEST
	PRINT
	P
	OTHER
	DB
	REDIS
	MEMCHD
	DB_OK
	DB_FAIL
	REDIS_OK
	REDIS_FAIL
	MEMCHD_OK
	MEMCHD_FAIL
)

type ColorFlag = int

const (
	NoColor ColorFlag = iota
	Color
)

type DestinationFlag = int

const (
	OFF_ALL DestinationFlag = iota
	ON_NO_COLOR
	ON_COLOR
	ON_ALL
)

type Destinations map[LogLvl][]io.Writer

// Default io.Writers Destinations
var defaultDestinations = Destinations{
	INFO:       {NoColor: ioutil.Discard, Color: ioutil.Discard},
	DEBUG:      {NoColor: ioutil.Discard, Color: ioutil.Discard},
	WARNING:    {NoColor: ioutil.Discard, Color: ioutil.Discard},
	ERROR:      {NoColor: ioutil.Discard, Color: ioutil.Discard},
	FATAL:      {NoColor: ioutil.Discard, Color: ioutil.Discard},
	TEST:       {NoColor: ioutil.Discard, Color: ioutil.Discard},
	PRINT:      {NoColor: ioutil.Discard, Color: ioutil.Discard},
	P:          {NoColor: ioutil.Discard, Color: ioutil.Discard},
	OTHER:      {NoColor: ioutil.Discard, Color: ioutil.Discard},
	DB:         {NoColor: ioutil.Discard, Color: ioutil.Discard},
	REDIS:      {NoColor: ioutil.Discard, Color: ioutil.Discard},
	MEMCHD:     {NoColor: ioutil.Discard, Color: ioutil.Discard},
	DB_OK:      {NoColor: ioutil.Discard, Color: ioutil.Discard},
	DB_FAIL:    {NoColor: ioutil.Discard, Color: ioutil.Discard},
	REDIS_OK:   {NoColor: ioutil.Discard, Color: ioutil.Discard},
	REDIS_FAIL: {NoColor: ioutil.Discard, Color: ioutil.Discard},
}

// Привязка обозначений цветовой схемы colormap к уровням логирования
var LoggerColorSchemeDetached = map[LogLvl]colormap.CSN{
	INFO:        colormap.CS_INFO,
	DEBUG:       colormap.CS_DEBUG,
	WARNING:     colormap.CS_WARNING,
	ERROR:       colormap.CS_ERROR,
	FATAL:       colormap.CS_FATAL_ERROR,
	TEST:        colormap.CS_TEST,
	PRINT:       colormap.CS_PRINT,
	P:           colormap.CS_PRINT,
	OTHER:       colormap.CS_PRINT,
	DB:          colormap.CS_DB,
	REDIS:       colormap.CS_REDIS,
	MEMCHD:      colormap.CS_MEMCHD,
	DB_OK:       colormap.CS_DB_OK,
	DB_FAIL:     colormap.CS_DB_FAIL,
	REDIS_OK:    colormap.CS_REDIS_OK,
	REDIS_FAIL:  colormap.CS_REDIS_FAIL,
	MEMCHD_OK:   colormap.CS_MEMCHD_OK,
	MEMCHD_FAIL: colormap.CS_MEMCHD_FAIL,
}

type LogHandler func(*logger, ...interface{}) // Func which execute in specific Log method (Info, Debug and etc.)
type LogMap map[LogLvl][]LogHandler

// Main struct of Logger
type logger struct {
	msgChan       chan logMsg  // Channel ready to write log msg
	settingsFlags int          // Global formatting logger settings
	delimiter     string       // Delimiters of columns (msg...)       TODO add param to NewLoggerFunc
	callDepth     int          // CallDepth Ignore value
	width         uint         // width pretty print (columns, msg..)  TODO
	pGen          func() int   // Gen sequences
	Destinations               // TwoD-slice of io.Writers
	LogMap                     // Map []LogHandlers
	colorMap      colormap.CSM // Color Map of Logger
}

// Log msg struct
type logMsg struct {
	LogLvl    // Log LogLvl msg
	ColorFlag // Features msg:  Color or NoColor msg
	string    // Msg for logging
}

// Generator Discard. Return LogHandler func which Discard log msg (Nothing to do)
func genDiscardFunc() LogHandler {
	return func(l *logger, msg ...interface{}) {
		return
	}
}

// Generator LogFunc. Return LogHandler func which print non color log
func genLogFunc(lvl LogLvl) LogHandler {
	return func(l *logger, msg ...interface{}) {
		l.log(lvl, NoColor, concatInterfaces(l.delimiter, msg...))
	}
}

// Generator ColorLogFunc. Return LogHandler func which print color log msg
func genColorLogFunc(lvl LogLvl, csn colormap.CSN) LogHandler {
	return func(l *logger, msg ...interface{}) {
		l.log(lvl, Color,
			colorConcatInterfaces(l.colorMap[csn], l.colorMap[colormap.CS_RESET][0], l.delimiter, msg...))
	}
}

// Goroutine func, Writer log msg by io.Writers info from lvlDestinations twoD-slice
func (l *logger) writeChanGoroutine() {
	for {
		if msg, ok := <-l.msgChan; ok { // канал закрыт
			io.WriteString(l.Destinations[msg.LogLvl][msg.ColorFlag], msg.string) // TODO Maybe optimized this by directly use syscall to write
		} else {
			break
		}
	}
}

func (l *logger) SetColorScheme(cs colormap.CSM) {
	newCS := make(colormap.CSM, len(cs))
	for i, v := range cs {
		newCS[i] = v
	}
	l.colorMap = newCS
}

func (l *logger) SetColorThemeName(name string) {
	l.SetColorScheme(colormap.CSMthemePicker(name))
}

func GetDefaultDestinations() (defaultDest Destinations) {
	defaultDest = make(Destinations, len(defaultDestinations))
	for lvl := range defaultDestinations {
		defaultDest[lvl] = make([]io.Writer, 2)
		copy(defaultDest[lvl], defaultDestinations[lvl])
	}
	return
}

func (l *logger) SetDefaultDestinations(defaultWriter io.Writer, flag DestinationFlag) {
	for lvl := range defaultDestinations {
		switch flag {
		case OFF_ALL:
			l.Destinations[lvl] = []io.Writer{NoColor: ioutil.Discard, Color: ioutil.Discard}
			l.LogMap[lvl] = []LogHandler{genDiscardFunc(), genDiscardFunc()}
		case ON_NO_COLOR:
			l.Destinations[lvl] = []io.Writer{NoColor: defaultWriter, Color: ioutil.Discard}
			l.LogMap[lvl] = []LogHandler{genLogFunc(lvl), genDiscardFunc()}
		case ON_COLOR:
			l.Destinations[lvl] = []io.Writer{NoColor: ioutil.Discard, Color: defaultWriter}
			l.LogMap[lvl] = []LogHandler{genDiscardFunc(), genColorLogFunc(lvl, LoggerColorSchemeDetached[lvl])}
		case ON_ALL:
			l.Destinations[lvl] = []io.Writer{NoColor: defaultWriter, Color: defaultWriter}
			l.LogMap[lvl] = []LogHandler{genLogFunc(lvl), genColorLogFunc(lvl, LoggerColorSchemeDetached[lvl])}
		}
	}
}

func (l *logger) SetDestinationLvl(lvl LogLvl, sWriters []io.Writer) {
	l.Destinations[lvl] = sWriters
}

func (l *logger) SetDestinationLvlColor(lvl LogLvl, color ColorFlag, writer io.Writer) {
	l.Destinations[lvl][color] = writer
}

func (l *logger) SetNewDestinations(destinations Destinations) {
	for lvl := range destinations {
		l.Destinations[lvl] = make([]io.Writer, 2)
		l.LogMap[lvl] = make([]LogHandler, 2)
		for color, writer := range destinations[lvl] {
			l.SetAndEnableDestinationLvlColor(lvl, color, writer)
		}
	}
}

func (l *logger) DisableDestinationLvl(lvl LogLvl) {
	l.LogMap[lvl][Color] = genDiscardFunc()
	l.LogMap[lvl][NoColor] = genDiscardFunc()
}

func (l *logger) DisableDestinationLvlColor(lvl LogLvl, color ColorFlag) {
	l.LogMap[lvl][color] = genDiscardFunc()
}

func (l *logger) EnableDestinationLvl(lvl LogLvl) {
	l.LogMap[lvl][Color] = genLogFunc(lvl)
	l.LogMap[lvl][NoColor] = genColorLogFunc(lvl, LoggerColorSchemeDetached[lvl]) // TODO CUSTOMIZE LoggerColorSchemeDetached
}

func (l *logger) EnableDestinationLvlColor(lvl LogLvl, color ColorFlag) {
	switch color {
	case NoColor:
		l.LogMap[lvl][color] = genLogFunc(lvl)
	case Color:
		l.LogMap[lvl][color] = genColorLogFunc(lvl, LoggerColorSchemeDetached[lvl]) // TODO CUSTOMIZE LoggerColorSchemeDetached
	}
}

func (l *logger) SetAndEnableDestinationLvl(lvl LogLvl, d []io.Writer) {
	l.SetDestinationLvl(lvl, d)
	l.EnableDestinationLvl(lvl)
}

func (l *logger) SetAndEnableDestinationLvlColor(lvl LogLvl, color ColorFlag, d io.Writer) {
	l.SetDestinationLvlColor(lvl, color, d)
	if d == ioutil.Discard {
		l.DisableDestinationLvlColor(lvl, color)
	} else {
		l.EnableDestinationLvlColor(lvl, color)
	}
}

func (l *logger) Log(lvl LogLvl, msg ...interface{}) {
	l.LogMap[lvl][NoColor](l, msg...)
	l.LogMap[lvl][Color](l, msg...)
}

func (l *logger) LogC(lvl LogLvl, cf ColorFlag, msg ...interface{}) {
	l.LogMap[lvl][cf](l, msg...)
}

func (l *logger) Info(msg ...interface{}) {
	l.LogMap[INFO][NoColor](l, msg...)
	l.LogMap[INFO][Color](l, msg...)
}

func (l *logger) Debug(msg ...interface{}) {
	l.LogMap[DEBUG][NoColor](l, msg...)
	l.LogMap[DEBUG][Color](l, msg...)
}

func (l *logger) Warning(msg ...interface{}) {
	l.LogMap[WARNING][NoColor](l, msg...)
	l.LogMap[WARNING][Color](l, msg...)
}

func (l *logger) Error(msg ...interface{}) {
	l.LogMap[ERROR][NoColor](l, msg...)
	l.LogMap[ERROR][Color](l, msg...)
}

func (l *logger) Fatal(msg ...interface{}) {
	l.LogMap[FATAL][NoColor](l, msg...)
	l.LogMap[FATAL][Color](l, msg...)
}

func (l *logger) Test(msg ...interface{}) {
	l.LogMap[TEST][NoColor](l, msg...)
	l.LogMap[TEST][Color](l, msg...)
}

func (l *logger) Print(msg ...interface{}) {
	l.LogMap[PRINT][NoColor](l, msg...)
	l.LogMap[PRINT][Color](l, msg...)
}

func (l *logger) P() {
	l.LogMap[INFO][NoColor](l, fmt.Sprint(l.pGen()))
	l.LogMap[INFO][Color](l, fmt.Sprint(l.pGen()))
}

func (l *logger) Other(msg ...interface{}) {
	l.LogMap[OTHER][NoColor](l, msg...)
	l.LogMap[OTHER][Color](l, msg...)
}

func concatInterfaces(delimiter string, msg ...interface{}) (result string) {
	for _, v := range msg {
		result = concat.Strings(result, fmt.Sprintf("%v", v))
		result = concat.Strings(result, delimiter)
	}
	return
}

func colorConcatInterfaces(cs colormap.ColorSheme, reset, delimiter string, msg ...interface{}) (s string) {
	lenCS := len(cs)
	for i, v := range msg {
		if lenCS > 0 {
			s = concat.Strings(s, cs[i])
			lenCS -= 1
		}
		s = concat.Strings(s, fmt.Sprintf("%v", v))
		s = concat.Strings(s, delimiter)
	}
	s = concat.Strings(s, reset)
	return s

}

func (l logger) log(lvl LogLvl, cf ColorFlag, msg string) {
	s := string(getSystemInfo(l.settingsFlags, l.callDepth+5)) // 5 - magic number - cnt LogLvl to func runtimeCaller()
	s = concat.Strings(s, msg)
	s = concat.Strings(s, "\n")
	l.msgChan <- logMsg{lvl, cf, s}
}

func (l *logger) Close() {
	if len(l.msgChan) == 0 {
		close(l.msgChan)
		return
	} else {
		t := time.NewTimer(time.Second)
		select {
		case <-t.C:
			close(l.msgChan)
			return
		default:
			if len(l.msgChan) == 0 {
				close(l.msgChan)
				return
			}
			time.Sleep(time.Millisecond * 10)
		}
	}
}

func getSystemInfo(settingsFlags int, callDepth int) (result []byte) {
	now := time.Now()
	file, line := "", 0
	if settingsFlags&LNoStackTrace == 0 {
		file, line = getRuntimeInfo(callDepth)
	}
	formatHeader(settingsFlags, &result, now, file, line)

	return result
}

func getRuntimeInfo(callDepth int) (string, int) {
	var ok bool
	_, file, line, ok := runtime.Caller(callDepth)
	if !ok {
		file = "???"
		line = 0
	}
	return file, line
}

// formatHeader writes log header to buf in following order:
//   * date and/or time (if corresponding flags are provided),
//   * file and line number (if corresponding flags are provided).
func formatHeader(settingsFlags int, buf *[]byte, t time.Time, file string, line int) {
	if settingsFlags&(Ldate|Ltime|Lmicroseconds) != 0 {
		if settingsFlags&LUTC != 0 {
			t = t.UTC()
		}
		if settingsFlags&Ldate != 0 {
			year, month, day := t.Date()
			itoa(buf, year, 4)
			*buf = append(*buf, '/')
			itoa(buf, int(month), 2)
			*buf = append(*buf, '/')
			itoa(buf, day, 2)
			*buf = append(*buf, ' ')
		}
		if settingsFlags&(Ltime|Lmicroseconds) != 0 {
			hour, min, sec := t.Clock()
			itoa(buf, hour, 2)
			*buf = append(*buf, ':')
			itoa(buf, min, 2)
			*buf = append(*buf, ':')
			itoa(buf, sec, 2)
			if settingsFlags&Lmicroseconds != 0 {
				*buf = append(*buf, '.')
				itoa(buf, t.Nanosecond()/1e3, 6)
			}
			*buf = append(*buf, ' ')
		}
	}
	if settingsFlags&LNoStackTrace == 0 {
		if settingsFlags&(Lshortfile|Llongfile) != 0 {
			if settingsFlags&Lshortfile != 0 {
				short := file
				for i := len(file) - 1; i > 0; i-- {
					if file[i] == '/' {
						short = file[i+1:]
						break
					}
				}
				file = short
			}
			*buf = append(*buf, file...)
			*buf = append(*buf, ':')
			itoa(buf, line, -1)
			*buf = append(*buf, ": "...)
		}
	}
}

// Cheap integer to fixed-width decimal ASCII. Give a negative width to avoid zero-padding.
func itoa(buf *[]byte, i int, wid int) {
	// Assemble decimal in reverse order.
	var b [20]byte
	bp := len(b) - 1
	for i >= 10 || wid > 1 {
		wid--
		q := i / 10
		b[bp] = byte('0' + i - q*10)
		bp--
		i = q
	}
	// i < 10
	b[bp] = byte('0' + i)
	*buf = append(*buf, b[bp:]...)
}
