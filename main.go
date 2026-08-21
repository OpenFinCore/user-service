package main

import (
	"fmt"

	"ofc-user-service/service"
)

func main() {
	fmt.Println("#==================================#")
	fmt.Println("#====== user-service ==========#")
	fmt.Println("#==================================#")

	container := service.NewServiceContainer()
	_ = container

	fmt.Println("========== Database ready, tables migrated & seeded ==========")
}
