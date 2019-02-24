package postgres

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/imperiuse/golib/concat"

	"github.com/imperiuse/golib/email"
	l "github.com/imperiuse/golib/logger"

	"github.com/jmoiron/sqlx"
	//nolint
	_ "github.com/lib/pq"
)

// SSL mod
const (
	DisableSSL    = "disable"
	RequireSSL    = "require"
	VerifyCASSL   = "verify-ca"
	VerifyFullSSL = "verify-full"
)

// PgDB - Bean for work with Postgres DB
type PgDB struct {
	Name string // Name DB (better uniq id in program)

	URL    string // Domain name (localhost - default)
	Host   string // Hostname domain (IP)
	Port   int    // Port Db (Postgres 5432)
	DbName string // Db name (main)

	SSL string // SSL mod (disable/enable)  @see https://godoc.org/github.com/lib/pq
	// * disable - No SSL * require - Always SSL (skip verification) * verify-ca - Always SSL * verify-full - Always SSL
	SSLCert     string // sslcert Cert file location. The file must contain PEM encoded data.
	SSLKey      string // sslkey Key file location. The file must contain PEM encoded data.
	SSLRootCert string // sslrootcert The location of the root certificate file. The file

	User string // The user to sign in as
	Pass string // The user's password

	CntAttemptRequest  int  // Cnt attempts connect to DB
	TimeAttemptRequest int  // Time between attempts  ! SECONDS !
	RepeatRequest      bool // Cnt try repeat execute SQL request to DB
	ConnMaxLifetime    int  // time in Nanosecond
	MaxIdleConns       int  // max idle connections
	MaxOpenConns       int  // max open connections

	Email  *email.MailBean // Email Bean for send error info
	Logger *l.LoggerI      // Pointer to Logger interface
	db     *sqlx.DB        // Pool connection to DB (return by sql.Open("postgres", ".....db_settings))
}

// GetDB - get current DB connect
func (pg *PgDB) GetDB() *sqlx.DB {
	return pg.db
}

// GetName - get Name obj DB
func (pg *PgDB) GetName() string {
	return pg.Name
}

// ConfigString - config connect DB
func (pg *PgDB) ConfigString() (config string) {
	switch pg.SSL {
	case VerifyFullSSL:
		fallthrough
	case VerifyCASSL:
		fallthrough
	case RequireSSL:
		config = fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s "+
			"sslmod=%s sslcert=%s sslkey=%s sslrootcert=%s",
			pg.Host, pg.Port, pg.DbName, pg.User, pg.Pass, pg.SSL, pg.SSLCert, pg.SSLKey, pg.SSLRootCert)
	case DisableSSL:
		fallthrough
	default:
		config = fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmod=%s ",
			pg.Host, pg.Port, pg.DbName, pg.User, pg.Pass, pg.SSL)
	}
	return
}

// Connect - Создание пула коннекшенов к БД
func (pg *PgDB) Connect() (err error) {
	// Use connect for "true" connect check to DB
	if pg.db, err = sqlx.Connect("postgres", pg.ConfigString()); err != nil {
		(*pg.Logger).Error("PgDB.Connect()", pg.Name, "Can't connect to DB server!",
			pg.ConfigString(), err)
	}
	if pg.ConnMaxLifetime > 0 {
		pg.db.SetConnMaxLifetime(time.Duration(pg.ConnMaxLifetime))
	}
	if pg.MaxIdleConns > 0 {
		pg.db.SetMaxIdleConns(pg.MaxIdleConns)
	}
	if pg.MaxOpenConns > 0 {
		pg.db.SetMaxOpenConns(pg.MaxOpenConns)
	}
	return err
}

// Close - Закрытие соединения
func (pg *PgDB) Close() {
	if err := pg.db.Close(); err != nil {
		(*pg.Logger).Error("PgDB.close()", pg.Name, "Can't close DB connection!", err)
	}
	(*pg.Logger).Info("PgDB.close()", pg.Name,
		concat.StringsMulti("Connection to database", pg.Host, ":", pg.DbName, "successful close()"))
}

func (pg *PgDB) executeDefer(callBy string, query string, err error, args ...interface{}) {
	if r := recover(); r != nil {
		(*pg.Logger).Error(callBy, pg.Name, "PANIC!", r)
		if err = pg.Email.SendEmails(
			fmt.Sprintf("PANIC!\n%v\nErr:\n%+v\nSQL:\n%v\nWith args:\n%+v", callBy, r, query, args)); err == nil {
			(*pg.Logger).Error(callBy, pg.Name, "Can't send email!", err)
		}
	}
}

// ExecuteQuery - Функция обертка над execute. Запросы с ожиданием данных от БД. (SELECT и т.д. возращающие значения)
func (pg *PgDB) ExecuteQuery(callBy string, query string, args ...interface{}) (rows *sql.Rows, err error) {
	defer pg.executeDefer(concat.Strings(callBy, " --> postgres.ExecuteQuery() [DEFER]"), query, err, args...)
	return pg.execute(true, concat.Strings(callBy, " --> postgres.execute(t)"), query, args...), err
}

// Execute - Функция обертка над execute. Запросы без ожидания данных от БД. (Update и т.д. не возращающие значения)
func (pg *PgDB) Execute(callBy string, query string, args ...interface{}) (err error) {
	defer pg.executeDefer(concat.Strings(callBy, " --> postgres.Execute() [DEFER]"), query, err, args...)
	_ = pg.execute(false, concat.Strings(callBy, " --> postgres.execute(f)"), query, args...)
	return err
}

// Функция выполнения запроса query
// @param
//     returnValue  bool        - флаг типа запроса
//     callBy       string      - кто вызвал, важно для логирования, чтобы не вызывать runtime.Caller()
//     query        string      - строка SQL запрос
//     args...      interface{} - аргументы
// @return
//     row          sql.Rows -  результат запроса, данные от БД
func (pg *PgDB) execute(returnValue bool, callBy string, query string, args ...interface{}) (row *sql.Rows) {
	var err error
	// Проверка коннекта к БД
	if err = pg.db.Ping(); err != nil {
		(*pg.Logger).Error(callBy, pg.Name, "Can't open connect (can't Ping) to Db server!", err)
		err = fmt.Errorf("no connect")
		panic(err)
	}
	for cnt := 0; cnt < pg.CntAttemptRequest; cnt++ {
		(*pg.Logger).Debug(callBy, pg.Name, concat.Strings("Attemp execute query: ", strconv.Itoa(cnt)))
		if returnValue { // TRUE == Execute_Query
			row, err = pg.db.Query(query, args...)
			if err != nil {
				(*pg.Logger).Log(l.DbFail, callBy, pg.Name, "SQL FAILED",
					err,
					query,
					"ARGS:", args)
			} else {
				(*pg.Logger).Log(l.DbOk, callBy, pg.Name, "SQL SUCCESS",
					query,
					"ARGS:", args)
				return row
			}
			time.Sleep(time.Duration(pg.TimeAttemptRequest) * time.Second)
		} else { // FALSE == Execute
			var results sql.Result
			results, err = pg.db.Exec(query, args...)
			if err != nil {
				(*pg.Logger).Log(l.DbFail, callBy, pg.Name, "SQL FAILED",
					err,
					query,
					"ARGS:", args)
			} else {
				(*pg.Logger).Log(l.DbOk, callBy, pg.Name, "SQL SUCCESS",
					query,
					"ARGS:", args)
				if rA, err1 := results.RowsAffected(); err1 == nil {
					(*pg.Logger).Debug("Rows affected:", rA)
				}
				return nil
			}
		}
	}
	(*pg.Logger).Error(callBy, pg.Name, "All try estimates! Panic!", err)
	panic(err)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
//////////////////////////////   Расширение SQLX              //////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// ExecuteQueryX - Функция обертка над QueryX (sqlX). Запросы с ожиданием данных от БД.
func (pg *PgDB) ExecuteQueryX(callBy string, query string, args ...interface{}) (rows *sqlx.Rows, err error) {
	defer pg.executeDefer(concat.Strings(callBy, " --> postgres.ExecuteQueryX() [DEFER]"), query, err, args...)
	return pg.executeX(concat.Strings(callBy, " --> postgres.executeX()"), query, args...), err
}

// Функция выполнения запроса queryX
// @param
//     callBy       string      - кто вызвал, важно для логирования, чтобы не вызывать runtime.Caller()
//     query        string      - строка SQL запрос
//     args...      interface{} - аргументы
//  @return
//     row          *sqlx.Rows
func (pg *PgDB) executeX(callBy string, query string, args ...interface{}) (row *sqlx.Rows) {
	var err error
	// Проверка коннекта к БД
	if err = pg.db.Ping(); err != nil {
		(*pg.Logger).Error(callBy, pg.Name, "Can't open connect (can't Ping) to Db server!", err)
		err = errors.New("no connect")
		panic(err)
	}
	for cnt := 0; cnt < pg.CntAttemptRequest; cnt++ {
		(*pg.Logger).Debug(callBy, pg.Name, concat.Strings("Attemp execute query: ", strconv.Itoa(cnt)))
		row, err = pg.db.Queryx(query, args...)
		if err != nil {
			(*pg.Logger).Log(l.DbFail, query, pg.Name, "SQL FAILED",
				err,
				query,
				"ARGS:", args)
		} else {
			(*pg.Logger).Log(l.DbOk, callBy, pg.Name, "SQL SUCCESS",
				query,
				"ARGS:", args)
			return row
		}
		time.Sleep(time.Duration(pg.TimeAttemptRequest) * time.Second)
	}
	(*pg.Logger).Error(callBy, pg.Name, "All try estimates! Panic!", err)
	panic(err)
}

// Select - syntax sugar of `SELECT ... ` method;
// @param
//     callBy       string      - кто вызвал, важно для логирования, чтобы не вызывать runtime.Caller()
//     dest         interface   - интерфейс указатель куда запишем данные
//     query        string      - строка SQL запрос
//     args...      interface{} - аргументы
//  @return
//                  error       - есть ли ошибка в запросе
func (pg *PgDB) Select(callBy string, dest interface{}, query string, args ...interface{}) error {
	// NOTE  // if you have null fields and use SELECT *, you must use sql.Null* in your struct
	if err := pg.db.Select(dest, query, args...); err != nil {
		(*pg.Logger).Log(l.DbFail, concat.Strings(callBy, " --> postgres.Select()"), pg.Name, "SQL FAILED", err,
			query,
			"ARGS:", args)
		return err
	} else {
		(*pg.Logger).Log(l.DbOk, concat.Strings(callBy, " --> postgres.Select()"), pg.Name, "SQL SUCCESS",
			query,
			"ARGS:", args)
		return nil
	}
}

// Get - syntax sugar of `SELECT ... LIMIT 1` method;
// @param
//     callBy       string      - кто вызвал, важно для логирования, чтобы не вызывать runtime.Caller()
//     dest         interface   - интерфейс указатель куда запишем данные
//     query        string      - строка SQL запрос
//     args...      interface{} - аргументы
//  @return
//                  error       - есть ли ошибка в запросе
func (pg *PgDB) Get(callBy string, dest interface{}, query string, args ...interface{}) error {
	if err := pg.db.Get(dest, query, args...); err != nil {
		(*pg.Logger).Log(l.DbFail, concat.Strings(callBy, " --> postgres.Get()"), pg.Name, "SQL FAILED", err,
			query,
			"ARGS:", args)
		return err
	} else {
		(*pg.Logger).Log(l.DbOk, concat.Strings(callBy, " --> postgres.Get()"), pg.Name, "SQL SUCCESS",
			query,
			"ARGS:", args)
		return nil
	}
}
