package util

import (
	"errors"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"gorm.io/gorm"
)

func DbIsDuplicatedErr(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return true
	}
	if err, ok := err.(*pgconn.PgError); ok {
		if err.Code == pgerrcode.UniqueViolation {
			return true
		}
	}
	return false
}

func DbIsNotFoundErr(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
