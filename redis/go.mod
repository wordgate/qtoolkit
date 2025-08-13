module github.com/wordgate/qtoolkit/redis

go 1.23.4

require (
	github.com/redis/go-redis/v9 v9.11.0
	github.com/spf13/viper v1.20.1
	github.com/wordgate/qtoolkit/core v0.0.0
)

replace github.com/wordgate/qtoolkit/core => ../core