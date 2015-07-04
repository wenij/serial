/*
Golang package for serial port

A Go package that allow you to read and write from the serial port.

This is a forked repo written by [@tarm](github.com/tarm).

Example usage:

  package main

  import (
    "time"
    "github.com/argandas/serial"
  )

  func main() {
    sp := serial.New()
    err := sp.Open("COM1", 9600)
    if err != nil {
      panic(err)
    }
    defer sp.Close()
    sp.Println("AT")
    sp.WaitForRegexTimeout("OK.*", time.Second * 10)
  }
*/

package serial
