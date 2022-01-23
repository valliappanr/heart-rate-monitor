/*
 * Copyright (c) Clinton Freeman 2015
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy of this software and
 * associated documentation files (the "Software"), to deal in the Software without restriction,
 * including without limitation the rights to use, copy, modify, merge, publish, distribute,
 * sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all copies or
 * substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT
 * NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
 * NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM,
 * DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

package main

import (
	"context"
	"fmt"
	"github.com/cfreeman/gatt"
	"github.com/cfreeman/gatt/examples/option"
	"github.com/gorilla/mux"
	"io"
	"io/ioutil"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"strconv"
	"sync/atomic"
)
const (
	expectedLastUpdatedTimeDuration = "-2m"
)
func retrieveLastUpdatedTime(w http.ResponseWriter, r *http.Request) {
	m, _ := time.ParseDuration(expectedLastUpdatedTimeDuration)
	var expectedLastUpdatedTime = time.Now().Add(m)
	var recentUpdatedTime = atomic.LoadInt64(&lastUpdatedTime)
	if expectedLastUpdatedTime.After(time.Unix(recentUpdatedTime, 0)) {
		shutdown(srv)
	}
	log.Println("last updated time:", lastUpdatedTime)
	updatedTimeString := strconv.FormatInt(lastUpdatedTime, 10)
	log.Println("updated time string", updatedTimeString)
	fmt.Fprint(w, updatedTimeString)
}

var lastUpdatedTime = time.Now().Unix()



var MeterUpdateUrl string

var srv http.Server


// onStateChanged is called when the bluetooth device changes state.
func onStateChanged(d gatt.Device, s gatt.State) {
	switch s {
	case gatt.StatePoweredOn:
		log.Println()
		log.Printf("INFO:SCANNING")
		log.Println()
		d.Scan([]gatt.UUID{}, false)
		return
	default:
		log.Printf("INFO: %s\n", s)
		log.Printf("INFO:STOPPED SCANNING")
		d.StopScanning()
	}
}

// onPeriphDiscovered is called when a new BLE peripheral is detected by the device.
func onPeriphDiscovered(p gatt.Peripheral, a *gatt.Advertisement, rssi int, deviceID string) {
	if strings.ToUpper(p.ID()) != strings.ToUpper(deviceID) {
		return // This is not the peripheral we're looking for, keep looking.
	}

	// Stop scanning once we've got the peripheral we're looking for.
	p.Device().StopScanning()

	log.Println()
	log.Println("INFO:CONNECTING")
	log.Println("INFO:  Peripheral ID     =", p.ID())
	log.Println("INFO:  Local Name        =", a.LocalName)
	log.Println("INFO:  TX Power Level    =", a.TxPowerLevel)
	log.Println("INFO:  Manufacturer Data =", a.ManufacturerData)
	log.Println("INFO:  Service Data      =", a.ServiceData)
	log.Println()

	// Connect to the peripheral once we have found it.
	p.Device().Connect(p)
}

// onPeriphConnected is called when we connect to a BLE peripheral.
func onPeriphConnected(p gatt.Peripheral, done chan bool, err error) {
	log.Println("INFO:CONNECTED")
	log.Println()

	defer p.Device().CancelConnection(p)

	if err := p.SetMTU(500); err != nil {
		log.Printf("ERROR: Failed to set MTU - %s\n", err)
	}

	// Get the heart rate service which is identified by the UUID: \x180d
	ss, err := p.DiscoverServices([]gatt.UUID{gatt.MustParseUUID("180d")})
	if err != nil {
		log.Printf("ERROR: Failed to discover services - %s\n", err)
		return
	}

	for _, s := range ss {
		// Get the heart rate measurement characteristic which is identified by the UUID: \x2a37
		cs, err := p.DiscoverCharacteristics([]gatt.UUID{gatt.MustParseUUID("2a37")}, s)
		if err != nil {
			log.Printf("ERROR: Failed to discover characteristics - %s\n", err)
			continue
		}

		for _, c := range cs {
			// Read the characteristic.
			if (c.Properties() & gatt.CharRead) != 0 {
				_, err := p.ReadCharacteristic(c)
				if err != nil {
					log.Printf("ERROR: Failed to read characteristic - %s\n", err)
					continue
				}
			}

			// Discover the characteristic descriptors.
			_, err := p.DiscoverDescriptors(nil, c)
			if err != nil {
				log.Printf("ERROR: Failed to discover descriptors - %s\n", err)
				continue
			}

			// Subscribe to any notifications from the characteristic.
			if (c.Properties() & (gatt.CharNotify | gatt.CharIndicate)) != 0 {

				err := p.SetNotifyValue(c, func(c *gatt.Characteristic, b []byte, err error) {
					heartRate := binary.LittleEndian.Uint16(append([]byte(b[1:2]), []byte{0}...))
					contact := binary.LittleEndian.Uint16(append([]byte(b[0:1]), []byte{0, 0}...))


					// Notify if the HRM has skin contact, and the current measured Heart rate.
					if contact == 6 || contact == 22 {
						fmt.Printf("1,%d\n", heartRate)
					} else {
						fmt.Printf("0,%d\n", heartRate)
					}
					meterData := MeterDataInput{Timestamp:CustomTime{time.Now()}, Value:float32(heartRate)}
					fmt.Println("meterData", meterData)
					atomic.StoreInt64(&lastUpdatedTime,time.Now().Unix())


					postBody, _ := json.Marshal(&meterData)
					reqBody := bytes.NewBuffer(postBody)
					response, err := http.Post(MeterUpdateUrl, "application/json", reqBody)
					fmt.Println("Posted response ", response)
					if (err != nil) {
						fmt.Println("err posting response", err)
					}
					if _, err := io.Copy(ioutil.Discard, response.Body); err != nil {
						log.Fatal(err)
					}
					response.Body.Close()
				})

				if err != nil {
					log.Printf("ERROR: Failed to subscribe characteristic - %s\n", err)
					continue
				}
			}

		}
		log.Println()
	}
	// Wait till we are disconnected from the HRM.
	<-done
	log.Println("Exiting from onPeriphConnected")
}

// onPeriphDisconnected is called when a BLE Peripheral is disconnected.
func onPeriphDisconnected(p gatt.Peripheral, done chan bool, s http.Server, err error) {
	shutdown(s)
	close(done)
	log.Println("INFO: Disconnected from BLE peripheral.")
}

func postChannel(mainChannel chan bool) {
	mainChannel <- true
}



func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func shutdown(s http.Server) {
	if err := s.Shutdown(context.Background()); err != nil {
		log.Fatal(err)
	}
}

// pollHeartRateMonitor connects to the BLE heart rate monitor at deviceID and
// collects heart rate measurements on the channel hr.
func main() {
	r := mux.NewRouter()

	r.HandleFunc("/lastUpdatedTime", retrieveLastUpdatedTime)

	srv := &http.Server{
		Handler:      r,
		Addr:         ":10003",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Println("Starting Server")
		if err := srv.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()


	f, err := os.OpenFile("/root/hrmmanager/hrm.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return // Ah-oh. Unable to log to file.
	}
	defer f.Close()
	log.SetOutput(f)

	deviceID :=getEnv("DEVICE_ID","00:00")
	fmt.Println("deviceID", deviceID)

	MeterUpdateUrl = "http://"+getEnv("METER_UPDATE_HOST", "localhost") + ":" +getEnv("METER_UPDATE_PORT", "10001") +"/updateMeterValue"
	fmt.Println("MeterUpdateUrl:", MeterUpdateUrl)

	handleBTDeviceManagement(deviceID, *srv)
}

func handleBTDeviceManagement(deviceID string, server http.Server)(err error) {
	done := make(chan bool)

	d, err := gatt.NewDevice(option.DefaultClientOptions...)
	if err != nil {
		log.Printf("ERROR: Unable to get bluetooth device.")
		shutdown(server)
		return err
	}
	fmt.Println("registering handler")

	// Register handlers.
	d.Handle(
		gatt.PeripheralDiscovered(func(p gatt.Peripheral, a *gatt.Advertisement, rssi int) {
			onPeriphDiscovered(p, a, rssi, deviceID)
		}),
		gatt.PeripheralConnected(func(p gatt.Peripheral, err error) {
			onPeriphConnected(p, done, err)
		}),
		gatt.PeripheralDisconnected(func(p gatt.Peripheral, err error) {
			onPeriphDisconnected(p, done, server, err)
		}),
	)

	d.Init(onStateChanged)

	// Wait till we are disconnected from the HRM.
	<-done

	return nil
}
