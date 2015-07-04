// Serialport package
//
// Author: Hugo Arganda
// Email: harganda@moreycorp.com
// Date: 07/01/2015
// 
// Copyright (C) 2013 The Morey Corporation
// 
// All rights reserved.  This file is the intellectual property of
// The Morey Corporation; it may not be copied, duplicated or
// transferred to a third party without prior written permission.

package serial

import (
	"fmt"
	"io"
	"time"
	"bytes"
	"regexp"
	"io/ioutil"
)

// Interval time to check if close port has been requested
const READ_TIMEOUT time.Duration = time.Millisecond * 100  

// End of line character (AKA EOL), newline character (ASCII 10, CR, '\n'). is used by default.
const EOL_DEFAULT byte = '\n'

/*******************************************************************************************
*******************************   TYPE DEFINITIONS 	****************************************
*******************************************************************************************/

type SerialPort struct {
	port io.ReadWriteCloser
	name string
	baud int
	portIsOpen bool
	rxChar chan byte
	rxLine chan string
	rxLineFlag bool
	closeReqChann chan bool
	closeAckChann chan error
	eol byte
	buff *bytes.Buffer
}

/*******************************************************************************************
********************************   BASIC FUNCTIONS  ****************************************
*******************************************************************************************/

func New() SerialPort {
	return SerialPort {
		eol: EOL_DEFAULT,
	}
}

func (sp *SerialPort) Open(name string, baud int, timeout ...time.Duration) (err error) {

	if !sp.portIsOpen {

		var readTimeout time.Duration 

		if len(timeout) > 0 {
			readTimeout = timeout[0]
		}

		comPort, e := openPort(name, baud, readTimeout)
		if e != nil {
			// Open port failed
			err = fmt.Errorf("Unable to open port \"%s\" - %s", name, e)
		} else {
			// Open port succesfull
			sp.name = name
			sp.baud = baud
			sp.port = comPort
			sp.portIsOpen = true
			sp.buff = bytes.NewBuffer(make([]byte, 256))
			sp.buff.Reset()
			// Open channels
			sp.rxChar = make(chan byte)
			sp.rxLine = make(chan string)
			sp.closeReqChann = make(chan bool)
			sp.closeAckChann = make(chan error)
			// Enable threads
			go sp.readSerialPort()
			go sp.processSerialPort()
		}
	} else {
		err = fmt.Errorf("\"%s\" is already open", name)
	}
	return
}

// This method close the current Serial Port.
func (sp *SerialPort) Close() (err error) {
	if sp.portIsOpen {
		sp.closeReqChann <- true
		err = <- sp.closeAckChann
		if err != nil {
			// Do nothing
		} else {
			sp.portIsOpen = false
			close(sp.rxChar)
			close(sp.rxLine)
		}
	}
	return
}

// This method prints data trough the serial port.
func (sp *SerialPort) Write(data []byte) (n int, err error) {
	if sp.portIsOpen {
		n , err = sp.port.Write(data)
		if err != nil {
			// Do nothing
		} else {
			sp.log("Tx", string(data))
		}
	} else {
		err = fmt.Errorf("Serial port is not open")
	}
	return
}

// This method prints data trough the serial port.
func (sp *SerialPort) Print(str string) error {
	if sp.portIsOpen {
		_ , err := sp.port.Write([]byte(str))
		if err != nil {
			return err
		} else {
			sp.log("Tx", str)
		}
	} else {
		return fmt.Errorf("Serial port is not open")
	}
	return nil
}

// Prints data to the serial port as human-readable ASCII text followed by a carriage return character 
// (ASCII 13, CR, '\r') and a newline character (ASCII 10, LF, '\n').
func (sp *SerialPort) Println(str string) error {
	return sp.Print(str + "\r\n")
}

// Printf formats according to a format specifier and print data trough the serial port.
func (sp *SerialPort) Printf(format string, args ...interface{}) error {
	str := format
	if len(args) > 0 {
		str = fmt.Sprintf(format, args...)
	}
	return sp.Print(str)
}

//This method send a binary file trough the serial port. If EnableLog is active then this method will log file related data.
func (sp *SerialPort) SendFile(filepath string) error {
	// Aux Vars
	sentBytes := 0
	q := 512
	data := []byte{}
	// Read file
	file, err := ioutil.ReadFile(filepath)
	if err != nil {	
		sp.log("DBG", "Invalid filepath" )
		return err
	} else {
		fileSize := len(file)
		sp.log("INF", "File size is %d bytes", fileSize )
		
		for sentBytes <= fileSize {
			//Try sending slices of less or equal than 512 bytes at time
			if len(file[sentBytes:]) > q {
				data = file[sentBytes:(sentBytes + q)]
			} else {
				data = file[sentBytes:]
			}
			// Write binaries
			_, err := sp.port.Write(data)
			if err != nil {
				sp.log("DBG", "Error while sending the file" )
				return err
			} else {
				sentBytes += q
				time.Sleep(time.Millisecond * 100)
			}
		}
	}
	//Encode data to send
	return nil
}

// Read the first byte of the serial buffer.
func (sp *SerialPort) Read() (byte, error) {
	if sp.portIsOpen {
		return sp.buff.ReadByte()
	} else {
		return 0x00, fmt.Errorf("Serial port is not open")
	}
	return 0x00, nil
}

// Read first available line from serial port buffer.
// 
// Line is delimited by the EOL character, newline character (ASCII 10, LF, '\n') is used by default.
// 
// The text returned from ReadLine does not include the line end ("\r\n" or '\n').
func (sp *SerialPort) ReadLine() (string, error) {
	if sp.portIsOpen {
		line, err := sp.buff.ReadString(sp.eol)
		if err != nil {
			return "", err
		} else {
			return removeEOL(line), nil
		}
	} else {
		return "", fmt.Errorf("Serial port is not open")
	}
	return "", nil
}

// Wait for a defined regular expression for a defined amount of time.
func (sp *SerialPort) WaitForRegexTimeout(exp string, timeout time.Duration) (string, error) {

	if sp.portIsOpen {
		//Decode received data
		timeExpired := false
	  	
		regExpPatttern := regexp.MustCompile(exp)

		//Timeout structure
		c1 := make(chan string, 1)
		go func() {
			sp.log("INF", "Waiting for RegExp:\r\n\t%s\r\n", exp)
			result := []string{}
			for !timeExpired {
				line, err := sp.ReadLine()
				if err != nil {
					// EOF wait tof fill buffer
					time.Sleep(time.Millisecond * 10)
				} else {
					result = regExpPatttern.FindAllString(line, -1)
					if len(result) > 0 {
						c1 <- result[0]
						break
					}
				}
			}
		}()
		select {
			case data := <-c1:
				format := "The RegExp: \r\n\t\t%s\r\n\tHas been matched: \r\n\t\t%s\r\n"
				sp.log("INF", format, exp, data)
				return data, nil
			case <-time.After(timeout):
				timeExpired = true
				return "", fmt.Errorf("Timeout expired")
				sp.log("INF", "Unable to match RegExp:\r\n\t%s\r\n", exp)
		}
	} else {
		return "", fmt.Errorf("Serial port is not open")
	} 
	return "", nil
}

// Available return the total number of available unread bytes on the serial buffer.
func (sp *SerialPort) Available() int {
	return sp.buff.Len()
}

// Change end of line character (AKA EOL), newline character (ASCII 10, LF, '\n') is used by default.
func (sp *SerialPort) EOL(c byte) {
	sp.eol = c
}

/*******************************************************************************************
******************************   PRIVATE FUNCTIONS  ****************************************
*******************************************************************************************/

func (sp *SerialPort) readSerialPort() {
	rxBuff := make([]byte, 256)
	for sp.portIsOpen {
		n, _ := sp.port.Read(rxBuff)
		// Write data to serial buffer
		sp.buff.Write(rxBuff[:n])
		for _, b := range rxBuff[:n] {
			sp.rxChar <- b
		}
		select {
			case <- sp.closeReqChann:
				sp.closeAckChann <- sp.port.Close()
				return
			default:
		}
	}
}

func (sp *SerialPort) processSerialPort() {
	screenBuff := make([]byte, 0)
	var lastRxByte byte
	for sp.portIsOpen {
		lastRxByte = <-sp.rxChar
		// Print received lines
		switch lastRxByte {
			case sp.eol:
				// EOL - Print received data
				sp.log("Rx", string(append(screenBuff, lastRxByte)))
				screenBuff = make([]byte, 0) //Clean buffer
				break
			default:
				screenBuff = append(screenBuff, lastRxByte)
		}
	}
}

func (sp *SerialPort) log(dir, data string, extras ...interface{}) {
	spacer := "-"
	if dir == "Tx" {
		spacer = ">>"
	} else {	
		if dir == "Rx" {
			spacer = "<<"
		}
	}
	if len(extras) > 0 {
		data = fmt.Sprintf(data, extras...)
	}
	fmt.Printf("[%s] %s %s %s\r\n", sp.name, dir, spacer, data)
	// logger.Log(typestr, label, data)
}

func removeEOL(line string) string {
	var data []byte
	// Remove CR byte "\r"
	for _, b := range []byte(line) {
		switch b {
			case '\r':
				// Do nothing
			case '\n':
				// Do nothing
			default:
				data = append(data, b)
		}
	}
	return string(data)
}

// REVISION LOG:
// mm/dd/yyyy | Author     			| Description
// ---------------------------------------
// 07/01/2015 | Hugo Arganda 		| First draft
// 07/02/2015 | Hugo Arganda 		| Printf method added