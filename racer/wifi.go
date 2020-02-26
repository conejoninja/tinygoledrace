package main

import (
	"machine"
	"strconv"
	"time"

	"tinygo.org/x/tinydraw"

	"tinygo.org/x/drivers/wifinina"
	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/proggy"

	"tinygo.org/x/drivers/net/mqtt"
)

const ssid = "YOURSSID"
const pass = "YOURPASS"

//const server = "ssl://test.mosquitto.org:8883"
const server = "tcp://test.mosquitto.org:1883"

var (
	/*uart = machine.UART1
	tx   = machine.UART_TX_PIN
	rx   = machine.UART_RX_PIN  */
	spi = machine.NINA_SPI

	// this is the ESP chip that has the WIFININA firmware flashed on it
	adaptor = &wifinina.Device{
		SPI:   spi,
		CS:    machine.NINA_CS,
		ACK:   machine.NINA_ACK,
		GPIO0: machine.NINA_GPIO0,
		RESET: machine.NINA_RESETN,
	}

	console = machine.UART0

	cl      mqtt.Client
	topicTx = "tinygo/tx"
	topicRx = "tinygo/rx"
	payload []byte
	enabled bool
)

func updateTrackInfo(client mqtt.Client, msg mqtt.Message) {
	// this code causes a hardfault    ¿?
	ba := msg.Payload()
	if len(ba) != 4 {
		return
	}
	var speed int16
	speed |= int16(ba[0])
	speed |= int16(ba[1]) << 8

	speedGaugeNeedle(speed, colors[BLACK])
	speedGaugeNeedle(speed, colors[player])
	oldSpeed = speed

	var progress int16
	progress |= int16(ba[2])
	progress |= int16(ba[3]) << 8
	resetLapBar()
	progressLapBar(progress)

	progress |= int16(ba[4])
	progress |= int16(ba[5]) << 8
	progressRaceBar(progress)

}

func configureWifi(player int) {
	display.FillScreen(colors[BACKGROUND])

	topicTx = "player" + strconv.Itoa(player) + "/tx"
	topicRx = "player" + strconv.Itoa(player) + "/rx"

	/*uart.Configure(machine.UARTConfig{TX: tx, RX: rx})
	rand.Seed(time.Now().UnixNano())*/

	// Configure SPI for 8Mhz, Mode 0, MSB First
	spi.Configure(machine.SPIConfig{
		Frequency: 8 * 1e6,
		MOSI:      machine.SPI0_MOSI_PIN,
		MISO:      machine.SPI0_MISO_PIN,
		SCK:       machine.SPI0_SCK_PIN,
	})

	// Init esp8266/esp32
	adaptor.Configure()
	connectToAP()

	opts := mqtt.NewClientOptions()
	opts.AddBroker(server).SetClientID("tinygo-racer-" + strconv.Itoa(player))

	println("Connecting to MQTT broker at", server)
	cl = mqtt.NewClient(opts)
	if token := cl.Connect(); token.Wait() && token.Error() != nil {
		failMessage(token.Error().Error())
		tinyfont.WriteLine(display, &proggy.TinySZ8pt7b, 0, 80, []byte(token.Error().Error()), colors[PLAYER1])
	}

	// subscribe
	token := cl.Subscribe(topicRx, 0, updateTrackInfo)
	token.Wait()
	if token.Error() != nil {
		tinyfont.WriteLine(display, &proggy.TinySZ8pt7b, 0, 90, []byte(token.Error().Error()), colors[PLAYER1])
		failMessage(token.Error().Error())
	}

	enabled = true

	go sendLoop()

	tinyfont.WriteLine(display, &proggy.TinySZ8pt7b, 0, 100, []byte("Done."), colors[PLAYER2])
	println("Done.")
}

// connect to access point
func connectToAP() {
	time.Sleep(2 * time.Second)
	tinyfont.WriteLine(display, &proggy.TinySZ8pt7b, 0, 30, []byte("Connecting to '"+ssid+"'"), colors[WHITE])
	println("Connecting to " + ssid)
	adaptor.SetPassphrase(ssid, pass)
	for st, _ := adaptor.GetConnectionStatus(); st != wifinina.StatusConnected; {
		display.FillRectangle(0, 31, 320, 12, colors[BACKGROUND])
		tinyfont.WriteLine(display, &proggy.TinySZ8pt7b, 0, 40, []byte(st.String()), colors[PLAYER1])
		println("Connection status: " + st.String())
		time.Sleep(1000 * time.Millisecond)
		st, _ = adaptor.GetConnectionStatus()
	}
	tinyfont.WriteLine(display, &proggy.TinySZ8pt7b, 0, 50, []byte("Connected :D"), colors[PLAYER2])
	println("Connected.")
	time.Sleep(2 * time.Second)
	ip, _, _, err := adaptor.GetIP()
	for ; err != nil; ip, _, _, err = adaptor.GetIP() {
		display.FillRectangle(0, 51, 320, 12, colors[BACKGROUND])
		tinyfont.WriteLine(display, &proggy.TinySZ8pt7b, 0, 60, []byte(err.Error()), colors[PLAYER1])
		println("IP", err.Error())
		time.Sleep(1 * time.Second)
	}
	tinyfont.WriteLine(display, &proggy.TinySZ8pt7b, 0, 70, []byte("IP: "+ip.String()), colors[PLAYER2])
	println(ip.String())
}

func failMessage(msg string) {
	tinyfont.WriteLine(display, &proggy.TinySZ8pt7b, 10, 200, []byte(msg), colors[PLAYER1])
	tinyfont.WriteLine(display, &proggy.TinySZ8pt7b, 10, 220, []byte("0xF1 ErrUnknowHost / 0xF3 ErrConnectionTimeout"), colors[STEPR])
	tinydraw.Rectangle(display, 4, 186, 312, 40, colors[PLAYER1])
	for {
		println(msg)
		time.Sleep(100 * time.Second)
	}
}

func sendLoop() {
	retries := uint8(0)
	var token mqtt.Token

	for {
		if enabled {
			if retries == 0 {
				println("Publishing MQTT message...", string(payload))
				token = cl.Publish(topicTx, 0, false, payload)
				token.Wait()
			}
			if retries > 0 || token.Error() != nil {
				if retries < 10 {
					token = cl.Connect()
					if token.Wait() && token.Error() != nil {
						retries++
						println("NOT CONNECTED TO MQTT (sendLoop)")
					} else {
						retries = 0
					}
				} else {
					enabled = false
				}
			}
			payload = []byte("none")
			time.Sleep(100 * time.Millisecond)
		} else {
			time.Sleep(1 * time.Second)
		}
	}
}

func Send(mqttpayload []byte) {
	payload = mqttpayload
}
