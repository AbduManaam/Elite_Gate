package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"elitegate/internal/model"
)

type ProjectRepo struct{
	BaseRepo
}

func NewProjectRepo(db *sql.DB)*ProjectRepo{
	return &ProjectRepo{BaseRepo{db:db}}
}

func(*ProjectRepo)Create(ctx context.Context,model *model.Project)error{

	tx,err:= r.db.BeginTx(ctx,nil)
	if err!=nil{
		
	}
}