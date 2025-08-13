module github.com/wordgate/qtoolkit/email

go 1.23.4

require (
	github.com/spf13/viper v1.20.1
	github.com/wordgate/qtoolkit/core v0.0.0
	gopkg.in/gomail.v2 v2.0.0-20160411212932-81ebce5c23df
)

replace github.com/wordgate/qtoolkit/core => ../core