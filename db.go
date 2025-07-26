package mods

import (
	"fmt"

	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var db *gorm.DB

func DB() *gorm.DB {
	return db
}

func initDb() {
	var err error
	dsn := viper.GetString("db")
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		fmt.Printf("Fatal error start: %v \n", err)
		panic(fmt.Sprintf("Fatal error start: %v \n", err))
	}

	if IsDev() {
		db = db.Debug()
	}
}
