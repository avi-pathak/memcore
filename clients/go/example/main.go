// Run against a local Memcore: go run .
package main

import (
	"fmt"
	"log"

	memcore "github.com/avinashpathak/memcore/clients/go"
)

func main() {
	c, err := memcore.Dial("127.0.0.1:6380")
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	if err := c.Ping(); err != nil {
		log.Fatal(err)
	}
	if err := c.Set("greeting", "hello"); err != nil {
		log.Fatal(err)
	}

	v, ok, err := c.Get("greeting")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("GET greeting -> %q (found=%v)\n", v, ok)

	n, err := c.Do("INCR", "counter")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("INCR counter -> %v\n", n)

	if _, err := c.Do("RPUSH", "fruits", "apple", "banana", "cherry"); err != nil {
		log.Fatal(err)
	}
	list, err := c.Do("LRANGE", "fruits", "0", "-1")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("LRANGE fruits -> %v\n", list)
}
