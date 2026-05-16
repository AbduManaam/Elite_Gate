package storage

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
)

func NewPostgres(logger zerolog.Logger) (*sql.DB, error) {
   
	dsn:= os.Getenv("POSTGRES_DSN")
	if dsn==" "{
		logger.Error().Msg("POSTGRES_DSN not set")
		return nil,fmt.Errorf("POSTGRES_DSN not set")
	}

	db,err:= sql.Open("postgres",dsn)
	if err!=nil{
		logger.Error().Err(err).Msg("Failed to open connection to DB")
		return nil,err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	if err:= db.Ping();err!=nil{
		logger.Error().Err(err).Msg("postgres ping failed")
		return nil,fmt.Errorf("postgres ping failed: %w", err)
	}
	logger.Info().Msg("Connected to Postgres")
	return db,nil
}

