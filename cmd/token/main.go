package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func main() {
	clientID := flag.String("client", "client-001", "client id to put in the token")
	role := flag.String("role", "client", "role to put in the token")
	secret := flag.String("secret", "super-secret-key", "JWT signing secret")
	ttl := flag.Duration("ttl", time.Hour, "token lifetime")
	flag.Parse()

	claims := jwt.MapClaims{
		"sub":  *clientID,
		"role": *role,
		"exp":  time.Now().Add(*ttl).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signed, err := token.SignedString([]byte(*secret))
	if err != nil {
		panic(err)
	}

	fmt.Println(signed)
}

//1. Generate JWT
//go run .\cmd\token -secret supersecretjwtkey
// Copy the token output.