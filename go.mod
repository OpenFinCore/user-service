module ofc-user-service

go 1.25.0

require (
	github.com/OpenFinCore/permissions-package v0.0.0
	github.com/google/uuid v1.6.0
	github.com/lib/pq v1.10.9
	go.uber.org/zap v1.27.0
	gorm.io/datatypes v1.2.5
	gorm.io/driver/postgres v1.5.11
	gorm.io/gorm v1.30.0
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/go-sql-driver/mysql v1.8.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20231201235250-de7065d80cb9 // indirect
	github.com/jackc/pgx/v5 v5.5.5 // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/crypto v0.22.0 // indirect
	golang.org/x/sync v0.9.0 // indirect
	golang.org/x/text v0.20.0 // indirect
	gorm.io/driver/mysql v1.5.6 // indirect
)

replace github.com/OpenFinCore/permissions-package => ../permissions
