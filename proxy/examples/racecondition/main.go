//go:build ignore

// Demonstração didática (NÃO faz parte do proxy de produção).
// Roda com:  go run -race examples/racecondition/main.go
// O -race detector aponta o data race no map compartilhado sem proteção.
package main

import (
	"fmt"
	"sync"
)

func main() {
	counts := map[string]int{} // map compartilhado, SEM mutex — propositalmente errado
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counts["chunks"]++ // concurrent map writes -> DATA RACE
		}()
	}
	wg.Wait()
	fmt.Println("counts:", counts)
}
