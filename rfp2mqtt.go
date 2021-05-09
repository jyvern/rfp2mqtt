/*
 * File: /home/jean-yves/go/src/rfp2mqtt/rfp2mqtt.go
 * Project: /home/jean-yves/go/src/rfp2mqtt
 * Created Date: Monday 23 March 2020
 * Author: Jean-Yves Vern
 * -----
 * Last Modified: Friday 28 August 2020 17:02:16
 * Modified By: the developer formerly known as Jean-Yves Vern at <jy.vern@laposte.net>
 * -----
 * Copyright (c) 2020 Company undefined
 * -----
 * HISTORY:
 */

package main

/**
 * Gateway package RFPlayer (RFP1000) to MQTT
 * Author : Jean-Yves Vern
 * Date : 28 mars 2019
 *
 * // ? Handle name and directory of configuration file in command line
 *
 * // ! Start daemon mode if set in config : https://github.com/VividCortex/godaemon
 * // ! Handle SIGTERM signal and CTRL/C : https://medium.com/@edwardpie/handling-os-signals-guarantees-graceful-degradation-in-go-cb57d604d39d
 * // ! Read certificate file from filesystem
 *
 * Update : 31 décembre 2019
 * 	- Ajout du support de connexion en TLS au broker (https://opium.io/blog/mqtt-in-go/)
 *
 * Update : 9 mai 2021
 *  - Ajout du support de commande x2d
 *  - Ajout de la donnée subtype dans le message json
 */

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang" // Communication with MQTT broker
	cache "github.com/patrickmn/go-cache"      // In memory structure to handle actuators and sensors
	log "github.com/sirupsen/logrus"           // For log facility
	conf "github.com/spf13/viper"              // Configuration handling

	rfp "github.com/jacobsa/go-serial/serial" // Communication with rfplayer dongle
)

/**
 *	Binary Data[]	Size	Type	Remark
 *	FrameType		1		byte	Value = 0
 *	DataFlag		1		byte	0: 433Mhz, 1: 868Mhz
 *	RFLevel			1		int8	Unit : dB  (high signal :-40dB to low : -110dB)
 *	FloorNoise		1		init : dB  (high signal :-40dB to low : -110dB)
 *	Protocol		1		byte below
 *	InfosType		1		byte below
 */

/* ******************************************************************** */

/**
 *	Binary Data[]	Size	Type	Remark
 *	FrameType		1		byte	Value = 0
 *	DataFlag		1		byte	0: 433Mhz, 1: 868Mhz
 *	RFLevel			1		int8	Unit : dB  (high signal :-40dB to low : -110dB)
 *	FloorNoise		1		int8	Unit : dB  (high signal :-40dB to low : -110dB)
 *	RFQuality		1		byte
 *	Protocol		1		byte	See below
 *	InfosType		1		byte	See below
 *	Infos[0?9]		20		Signed or uint16
 *	upon context	LSB first. Define provided data by the device
 */

const sync1ContainerConstant byte = 'Z'
const sync2ContainerConstant byte = 'I'
const qualifierASCIIInterpretor byte = '+'

//const  QualifierASCIIConstant2 byte =  ':'

const asciiContainerMask byte = 0x40

const regularIncomingBinaryUSBFrameType = 0
const regularIncomngRfBinaryUSBFrameType = 0

const rflinkIncomingBinaryUSBFrameType = 1
const rflinkIncomingRfBinaryUSBFrameType = 1

const sourceDest433868 = 1

const sendActionOFF = 0
const sendActionON = 1
const sendActionDIM = 2
const sendActionBRIGHT = 3
const sendActionALLOFF = 4
const sendActionALLON = 5
const sendActionASSOC = 6
const sendActionDISSOC = 7
const sendActionASSOCOFF = 8
const sendActionDISSOCOFF = 9

/* ***************************************** */

const sendUNDEFINEDProtocol = 0
const sendVISONICProtocol433 = 1
const sendVISONICProtocol868 = 2
const sendCHACONProtocol433 = 3
const sendDOMIAProtocol433 = 4
const sendX10Protocol433 = 5
const sendX2DProtocol433 = 6
const sendX2DProtocol868 = 7
const sendX2DSHUTTERProtocol868 = 8
const sendX2DHAELECProtocol868 = 9
const sendX2DHAGASOILProtocol868 = 10
const sendSOMFYProtocol433 = 11
const sendBLYSSProtocol433 = 12
const sendPARROT = 13
const sendOREGONProtocol433X = 14     /* not reachable by API */
const sendOWL433X = 15                /* not reachable by API */
const sendKD101Protocol433 = 16       /* 32 bits ID */
const sendDIGIMAXTS10Protocol433 = 17 /* deprecated */
const sendOREGONProtocolV1433 = 18    /* not reachable by API */
const sendOREGONProtocolV2433 = 19    /* not reachable by API */
const sendOREGONProtocolV3433 = 20    /* not reachable by API */
const sendTIC433 = 21                 /* not reachable by API */
const sendFS20868 = 22

/* ***************************************** */

const receivedProtocolUNDEFINED = 0
const receivedProtocolX10 = 1
const receivedProtocolVISONIC = 2
const receivedProtocolBLYSS = 3
const receivedProtocolCHACON = 4
const receivedProtocolOREGON = 5
const receivedProtocolDOMIA = 6
const receivedProtocolOWL = 7
const receivedProtocolX2D = 8
const receivedProtocolRFY = 9
const receivedProtocolKD101 = 10
const receivedProtocolPARROT = 11
const receivedProtocolDIGIMAX = 12 /* deprecated */
const receivedProtocolTIC = 13
const receivedProtocolFS20 = 14
const receivedProtocolJAMMING = 15

const regularIncomingRFBinaryUSBFrameInfosWordsNumber = 10

const infosType0 = 0
const infosType1 = 1
const infosType2 = 2
const infosType3 = 3
const infosType4 = 4
const infosType5 = 5
const infosType6 = 6
const infosType7 = 7
const infosType8 = 8
const infosType9 = 9
const infosType10 = 10
const infosType11 = 11
const infosType12 = 12 /* deprecated */
const infosType13 = 13
const infosType14 = 14
const infosType15 = 15

// Sensor : Struct for sensors
type Sensor struct {
	Ref      string
	SubType  string
	Name     string
	Protocol string
	Topic    string
}

type sensors []Sensor

type payloadTH struct {
	T string
	H string
}

type messageContainerHeader struct {
	sync1               byte
	sync2               byte
	sourceDestQualifier byte
	qualifierOrLenLsb   byte
	qualifierOrLenMsb   byte
}

type regularIncomingBinaryUSBFrame struct { // public binary API USB to RF frame Sending
	frameType byte
	cluster   byte // set 0 by default. Cluster destination.
	protocol  byte
	action    byte
	ID        uint32 // LSB first. Little endian, mostly 1 byte is used
	dimValue  byte
	burst     byte
	qualifier byte
	reserved2 byte // set 0
}

type incomingRFInfosType0 struct { // used by X10 / Domia Lite protocols
	subType uint16
	id      uint16
}

type incomingRFInfosType1 struct { // Used by X10 (32 bits ID) and CHACON
	subType uint16
	idLsb   uint16
	idMsb   uint16
}

type incomingRFInfosType2 struct { // Used by  VISONIC /Focus/Atlantic/Meian Tech
	subType   uint16
	idLsb     uint16
	idMsb     uint16
	qualifier uint16
}

type incomingRFInfosType3 struct { //  Used by RFY PROTOCOL
	subType   uint16
	idLsb     uint16
	idMsb     uint16
	qualifier uint16
}

type incomingRFInfosType4 struct { // Used by  Scientific Oregon  protocol ( thermo/hygro sensors)
	subType   uint16
	idPHY     uint16
	idChannel uint16
	qualifier uint16
	temp      int16  // UNIT:  1/10 of degree Celsius
	hygro     uint16 // 0...100  UNIT: %
}

type incomingRFInfosType5 struct { // Used by  Scientific Oregon  protocol  ( Atmospheric  pressure  sensors)
	subType   uint16
	idPHY     uint16
	idChannel uint16
	qualifier uint16
	temp      int16  // UNIT:  1/10 of degree Celsius
	hygro     uint16 // 0...100  UNIT: %
	pressure  uint16 //  UNIT: hPa
}

type incomingRFInfosType6 struct { // Used by  Scientific Oregon  protocol  (Wind sensors)
	subType   uint16
	idPHY     uint16
	idChannel uint16
	qualifier uint16
	speed     uint16 // Averaged Wind speed   (Unit : 1/10 m/s, e.g. 213 means 21.3m/s)
	direction uint16 // Wind direction  0-359° (Unit : angular degrees)
}

type incomingRFInfosType7 struct { // Used by  Scientific Oregon  protocol  ( UV  sensors)
	subType   uint16
	idPHY     uint16
	idChannel uint16
	qualifier uint16
	light     uint16 // UV index  1..10  (Unit : -)
}

type incomingRFInfosType8 struct { // Used by  OWL  ( Energy/power sensors)
	subType   uint16
	idPHY     uint16
	idChannel uint16
	qualifier uint16
	energyLsb uint16 // LSB: energy measured since the RESET of the device  (32 bits value). Unit : Wh
	energyMsb uint16 // MSB: energy measured since the RESET of the device  (32 bits value). Unit : Wh
	power     uint16 // Instantaneous measured (total)power. Unit : W (with U=230V, P=P1+P2+P3))
	powerI1   uint16 // Instantaneous measured at input 1 power. Unit : W (with U=230V, P1=UxI1)
	powerI2   uint16 // Instantaneous measured at input 2 power. Unit : W (with U=230V, P2=UxI2)
	powerI3   uint16 // Instantaneous measured at input 3 power. Unit : W (with U=230V, P2=UxI3)
}

type incomingRFInfosType9 struct { // Used by  OREGON  ( Rain sensors)
	subType      uint16
	idPHY        uint16
	idChannel    uint16
	qualifier    uint16
	totalRainLsb uint16 // LSB: rain measured since the RESET of the device  (32 bits value). Unit : 0.1 mm
	totalRainMsb uint16 // MSB: rain measured since the RESET of the device
	rain         uint16 // Instantaneous measured rain. Unit : 0.01 mm/h
}

type incomingRFInfosType10 struct { // Used by Thermostats  X2D protocol
	subType   uint16
	idLsb     uint16
	idMsb     uint16
	qualifier uint16 // D0 : Tamper Flag, D1: Alarm Flag, D2: Low Batt Flag, D3: Supervisor Frame, D4: Test  D6:7 : X2D variant
	function  uint16
	mode      uint16    //
	data      [4]uint16 // provision
}

type incomingRFInfosType11 struct { // Used by Alarm/remote control devices  X2D protocol
	subType   uint16
	idLsb     uint16
	idMsb     uint16
	qualifier uint16 // D0 : Tamper Flag, D1: Alarm Flag, D2: Low Batt Flag, D3: Supervisor Frame, D4: Test  D6:7 : X2D variant
	reserved1 uint16
	reserved2 uint16
	data      [4]uint16 //  provision
}

type incomingRFInfosType12 struct { // Used by  DIGIMAX TS10 protocol // deprecated
	subType   uint16
	idLsb     uint16
	idMsb     uint16
	qualifier uint16
	temp      int16 // UNIT:  1/10 of degree
	setPoint  int16 // UNIT:  1/10 of degree
}

type incomingRFInfosType13 struct { // Used by  Cartelectronic TIC/Pulses devices (Teleinfo/TeleCounters)
	subtype       uint16 // subtype/version
	idLsb         uint16
	idMsb         uint16
	qualifier     uint16 // D8-15: idMsb2  D0:7 : flags
	infos         uint16 // state/contract type
	counter1Lsb   uint16
	counter1Msb   uint16
	counter2Lsb   uint16
	counter2Msb   uint16
	apparentPower uint16
}

type incomingRFInfosType14 struct { // Used by FS20. Same file as INCOMING_RF_INFOS_TYPE2
	subtype   uint16
	idLsb     uint16
	idMsb     uint16
	qualifier uint16
}

type incomingRFInfosType15 struct { // Used by JAMMING (idem Type1)
	subType uint16
	idLsb   uint16
	idMsb   uint16
}

var cmqtt mqtt.Client
var cmqttOpts mqtt.ClientOptions

var b bytes.Buffer
var ch chan []byte

// var insecure *bool

var rfpConfig rfp.OpenOptions
var rfpPort io.ReadWriteCloser

var errGlobal error
var sensorsNameCache *cache.Cache       // Indexed by Id
var sensorsTopicCache *cache.Cache      // Indexed by Id
var actuatorsIDCache *cache.Cache       // Indexed by Name
var actuatorsTopicCache *cache.Cache    // Indexed by Name
var actuatorsCommandCache *cache.Cache  // Indexed by Name
var actuatorsProtocolCache *cache.Cache // Indexed by Name

var iCompteur int

var iWait2Send int

var flagConfigFile string

// Config : Internal struct type for config datas described in config.yml
type Config struct {
	Rfplayer struct {
		WaitToSend        int    `yaml:"waittosend"`
		Port              string `yaml:"port"`
		Timeout           int    `yaml:"timeout"`
		Minread           int    `yaml:"minread"`
		RTSCTSFlowControl bool   `yaml:"rtsctsflowcontrol"`
		Initialisation    []struct {
			Cmd string `yaml:"cmd"`
		} `yaml:"initialisation"`
	} `yaml:"rfplayer"`
	Brockermqtt struct {
		Username  string `yaml:"username"`
		Password  string `yaml:"password"`
		Protocol  string `yaml:"protocol"`
		Address   string `yaml:"address"`
		Port      int    `yaml:"port"`
		Certfile  string `yaml:"certfile"`
		Insecure  bool   `yaml:"insecure"`
		TopicRoot string `yaml:"topicroot"`
	} `yaml:"brockermqtt"`
	Log struct {
		Format string `yaml:"format"`
		Output string `yaml:"output"`
		Level  string `yaml:"level"`
	} `yaml:"log"`
	Sensors []struct {
		ID    string `yaml:"id"`
		Name  string `yaml:"nom"`
		Ref   string `yaml:"ref,omitempty"`
		Topic string `yaml:"topic,omitempty"`
	} `yaml:"sensors"`
	Actuators []struct {
		ID       string `yaml:"id"`
		Name     string `yaml:"name"`
		Protocol string `yaml:"protocol"`
		Topic    string `yaml:"topic"`
		Command  string `yaml:"command"`
	} `yaml:"actuators"`
}

var config Config

/**
 * Compute an unsigned 32 bits integer from unsigned 16 bits integer
 */
func touint32(msb uint16, lsb uint16) (u32 uint32) {
	u32 = uint32(msb) << 16
	u32 = u32 + uint32(lsb)

	log.Debug("touint32: ", msb, " / ", lsb, " => ", u32)

	return u32
}

/**
 * Compute an unsigned 32 bits integer from an ascii deviceIO ie X10 code (A1, C2, ...)
 */
func atobDeviceID(dID string) (u32 uint32) {

	uLettre := dID[0] - 'A'
	// sChiffre := dID[1:len(dID)]
	sChiffre := dID[1:]
	uChiffre, err := strconv.ParseUint(sChiffre, 10, 32)
	if err != nil {
		return 0
	}

	u32 = (uint32(uLettre) * 16) + uint32(uChiffre) - 1

	log.Debug("atobDeviceID: ", dID, " => ", u32, " => character: ", string(dID[0]), ", number: ", uChiffre)

	return u32
}

/**
 * Function that return a string 0 or 1 corresponding to the bit npar of byte bpar
 */
func testBit(bpar byte, npar int) string {
	if bpar&(1<<uint8(npar)) != 0 {
		return "1"
	}
	return "0"
}

/**
 * Decode a message from RFPlayer
 */
func decode(l int, m []byte) {
	var jsonString string

	timecodeString := time.Now().Format(time.RFC3339)

	sensor := Sensor{}

	log.Debug("RFLevel=", int8(m[8]), ", FloorNoise=", int8(m[9]), ", RFQuality=", m[10], ", Protocol=", m[11], ", InfosType=", m[12])

	switch m[12] {
	case infosType0:
		log.Debug(", X10, DOMIA_LITE, PARROT")
		log.Debug(", SubType=", binary.LittleEndian.Uint16(m[13:]))
		log.Debug(", Id=", binary.LittleEndian.Uint32(m[15:]))

		sensor.Ref = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[13:])), 10)
		sensor.Protocol = "X10"
		sensor.SubType = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[13:])), 10)
		sensor.Name = sensorName(sensor.Ref)
		sensor.Topic = sensorTopic(sensor.Ref)
		if sensor.Topic == "NULL" {
			sensor.Topic = conf.GetString("brokermqtt.topicroot") + "/" + sensor.Ref + "/x10"
		}
		log.Debug(", topic=", sensor.Topic)

		topicSplit := strings.Split(sensor.Topic, "/")

		jsonString = "{ \"tc\": \"" + timecodeString
		jsonString = jsonString + "\" , \"n\": \"" + topicSplit[1]
		jsonString = jsonString + "\" , \"r\": \"" + sensor.Ref
		jsonString = jsonString + "\" , \"st\": \"" + sensor.SubType
		jsonString = jsonString + "\" }"

	case infosType1:
		log.Debug(", CHACON ...")
		log.Debug(", SubType=", binary.LittleEndian.Uint16(m[13:]))
		log.Debug(", Id=", binary.LittleEndian.Uint32(m[15:]))

		sensor.Ref = "1-" + strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[15:])), 10)
		sensor.Protocol = "CHACON"
		sensor.SubType = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[13:])), 10)
		sensor.Name = sensorName(sensor.Ref)
		sensor.Topic = sensorTopic(sensor.Ref)
		if sensor.Topic == "NULL" {
			sensor.Topic = conf.GetString("brokermqtt.topicroot") + "/" + sensor.Ref + "/chacon"
		}
		log.Debug(", topic=", sensor.Topic)

		topicSplit := strings.Split(sensor.Topic, "/")

		jsonString = "{ \"tc\": \"" + timecodeString
		jsonString = jsonString + "\" , \"n\": \"" + topicSplit[1]
		jsonString = jsonString + "\" , \"r\": \"" + sensor.Ref
		jsonString = jsonString + "\" , \"st\": \"" + sensor.SubType
		jsonString = jsonString + "\" }"

	case infosType2:
		log.Debug(", VISONIC")
		log.Debug(", SubType=", binary.LittleEndian.Uint16(m[13:]))
		log.Debug(", Id=", binary.LittleEndian.Uint32(m[15:]))
		log.Debug(", qualifier=", binary.LittleEndian.Uint16(m[19:]))

		sensor.Ref = "2-" + strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[15:])), 10)
		sensor.Protocol = "VISONIC"
		sensor.SubType = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[13:])), 10)
		sensor.Name = sensorName(sensor.Ref)
		sensor.Topic = sensorTopic(sensor.Ref)
		if sensor.Topic == "NULL" {
			sensor.Topic = conf.GetString("brokermqtt.topicroot") + "/" + sensor.Ref + "/visonic"
		}
		log.Debug(", topic=", sensor.Topic)

		qualifierString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[19:])), 10)

		topicSplit := strings.Split(sensor.Topic, "/")

		jsonString = "{ \"tc\": \"" + timecodeString
		jsonString = jsonString + "\" , \"n\": \"" + topicSplit[1]
		jsonString = jsonString + "\" , \"r\": \"" + sensor.Ref
		jsonString = jsonString + "\" , \"q\": \"" + qualifierString
		jsonString = jsonString + "\" , \"ftamper\": \"" + testBit(m[19], 0)  // tamper flag
		jsonString = jsonString + "\" , \"falarm\": \"" + testBit(m[19], 1)   // alarm flag
		jsonString = jsonString + "\" , \"flowbatt\": \"" + testBit(m[19], 2) // low batt flag
		jsonString = jsonString + "\" , \"falive\": \"" + testBit(m[19], 3)   // supervisor message flag
		jsonString = jsonString + "\" , \"st\": \"" + sensor.SubType
		jsonString = jsonString + "\" }"

	case infosType3:
		log.Debug(", RTS")
		log.Debug(", SubType=", binary.LittleEndian.Uint16(m[13:]))
		log.Debug(", Id=", binary.LittleEndian.Uint32(m[15:]))
		log.Debug(", qualifier=", binary.LittleEndian.Uint16(m[19:]))

		sensor.Ref = "3-" + strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[15:])), 10)
		sensor.Protocol = "RTS"
		sensor.SubType = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[13:])), 10)
		sensor.Name = sensorName(sensor.Ref)
		sensor.Topic = sensorTopic(sensor.Ref)
		if sensor.Topic == "NULL" {
			sensor.Topic = conf.GetString("brokermqtt.topicroot") + "/" + sensor.Ref + "/rts"
		}
		log.Debug(", topic=", sensor.Topic)

		qualifierString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[19:])), 10)

		topicSplit := strings.Split(sensor.Topic, "/")

		jsonString = "{ \"tc\": \"" + timecodeString
		jsonString = jsonString + "\" , \"n\": \"" + topicSplit[1]
		jsonString = jsonString + "\" , \"r\": \"" + sensor.Ref
		jsonString = jsonString + "\" , \"q\": \"" + qualifierString
		jsonString = jsonString + "\" , \"st\": \"" + sensor.SubType
		jsonString = jsonString + "\" }"

	case infosType4:
		log.Debug(", OREGON Thermo/Hygro")
		log.Debug(", SubType=", binary.LittleEndian.Uint16(m[13:]))
		log.Debug(", idPHY=", binary.LittleEndian.Uint16(m[15:]))
		log.Debug(", idChannel=", binary.LittleEndian.Uint16(m[17:]))
		log.Debug(", qualifier=", binary.LittleEndian.Uint16(m[19:]))
		log.Debug(", temp=", int16(binary.LittleEndian.Uint16(m[21:])))
		log.Debug(", hygro=", binary.LittleEndian.Uint16(m[23:]))

		sensor.Ref = "4-" + strconv.FormatUint(uint64(touint32(binary.LittleEndian.Uint16(m[15:]), binary.LittleEndian.Uint16(m[17:]))), 10)
		sensor.Protocol = "OREGON"
		sensor.SubType = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[13:])), 10)
		sensor.Name = sensorName(sensor.Ref)
		sensor.Topic = sensorTopic(sensor.Ref)
		if sensor.Topic == "NULL" {
			sensor.Topic = conf.GetString("brokermqtt.topicroot") + "/" + sensor.Ref + "/th"
		}
		log.Debug(", topic=", sensor.Topic)

		tempString := strconv.FormatFloat(float64(uint64(binary.LittleEndian.Uint16(m[21:])))*0.1, 'f', 1, 64)
		humiString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[23:])), 10)

		topicSplit := strings.Split(sensor.Topic, "/")

		jsonString = "{ \"tc\": \"" + timecodeString
		jsonString = jsonString + "\" , \"n\": \"" + topicSplit[1]
		jsonString = jsonString + "\" , \"r\": \"" + sensor.Ref
		jsonString = jsonString + "\" , \"t\": \"" + tempString
		jsonString = jsonString + "\" , \"h\": \"" + humiString
		jsonString = jsonString + "\" , \"flowbatt\": \"" + testBit(m[19], 0) // low batt flag
		jsonString = jsonString + "\" , \"st\": \"" + sensor.SubType
		jsonString = jsonString + "\" }"
		//		} else {
		//			log.Info("RFLevel=", int8(m[8]), ", FloorNoise=", int8(m[9]), ", RFQuality=", m[10], ", Protocol=", m[11], ", InfosType=", m[12])
		//			log.Info("Topic problem : topic=>", sensor.Topic, "<, len=", len(topicSplit), ", Sensor Ref:>", sensor.Ref, "<")
		//		}

	case infosType5:
		log.Debug(", OREGON Atmo pressure")
		log.Debug(", SubType=", binary.LittleEndian.Uint16(m[13:]))
		log.Debug(", idPHY=", binary.LittleEndian.Uint16(m[15:]))
		log.Debug(", idChannel=", binary.LittleEndian.Uint16(m[17:]))
		log.Debug(", qualifier=", binary.LittleEndian.Uint16(m[19:]))
		log.Debug(", temp=", int16(binary.LittleEndian.Uint16(m[21:])))
		log.Debug(", hygro=", binary.LittleEndian.Uint16(m[23:]))
		log.Debug(", pressure=", binary.LittleEndian.Uint16(m[25:]))

		sensor.Ref = "5-" + strconv.FormatUint(uint64(touint32(binary.LittleEndian.Uint16(m[15:]), binary.LittleEndian.Uint16(m[17:]))), 10)
		sensor.Protocol = "OREGON"
		sensor.SubType = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[13:])), 10)
		sensor.Name = sensorName(sensor.Ref)
		sensor.Topic = sensorTopic(sensor.Ref)
		if sensor.Topic == "NULL" {
			sensor.Topic = conf.GetString("brokermqtt.topicroot") + "/" + sensor.Ref + "/thpa"
		}
		log.Debug(", topic=", sensor.Topic)

		tempString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[21:])), 10)
		humiString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[23:])), 10)
		pressureString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[25:])), 10)

		topicSplit := strings.Split(sensor.Topic, "/")

		jsonString = "{ \"tc\": \"" + timecodeString
		jsonString = jsonString + "\" , \"n\": \"" + topicSplit[1]
		jsonString = jsonString + "\" , \"r\": \"" + sensor.Ref
		jsonString = jsonString + "\" , \"t\": \"" + tempString
		jsonString = jsonString + "\" , \"h\": \"" + humiString
		jsonString = jsonString + "\" , \"p\": \"" + pressureString
		jsonString = jsonString + "\" , \"flowbatt\": \"" + testBit(m[19], 0) // low batt flag
		jsonString = jsonString + "\" , \"st\": \"" + sensor.SubType
		jsonString = jsonString + "\" }"

	case infosType6:
		log.Debug(", OREGON Wind")
		log.Debug(", SubType=", binary.LittleEndian.Uint16(m[13:]))
		log.Debug(", idPHY=", binary.LittleEndian.Uint16(m[15:]))
		log.Debug(", idChannel=", binary.LittleEndian.Uint16(m[17:]))
		log.Debug(", qualifier=", binary.LittleEndian.Uint16(m[19:]))
		log.Debug(", speed=", binary.LittleEndian.Uint16(m[21:]))
		log.Debug(", direction=", binary.LittleEndian.Uint16(m[23:]))

		sensor.Ref = "6-" + strconv.FormatUint(uint64(touint32(binary.LittleEndian.Uint16(m[15:]), binary.LittleEndian.Uint16(m[17:]))), 10)
		sensor.Protocol = "OREGON"
		sensor.SubType = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[13:])), 10)
		sensor.Name = sensorName(sensor.Ref)
		sensor.Topic = sensorTopic(sensor.Ref)
		if sensor.Topic == "NULL" {
			sensor.Topic = conf.GetString("brokermqtt.topicroot") + "/" + sensor.Ref + "/wind"
		}
		log.Debug(", topic=", sensor.Topic)

		speedString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[21:])), 10)
		directionString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[23:])), 10)

		topicSplit := strings.Split(sensor.Topic, "/")

		jsonString = "{ \"tc\": \"" + timecodeString
		jsonString = jsonString + "\" , \"n\": \"" + topicSplit[1]
		jsonString = jsonString + "\" , \"r\": \"" + sensor.Ref
		jsonString = jsonString + "\" , \"s\": \"" + speedString
		jsonString = jsonString + "\" , \"d\": \"" + directionString
		jsonString = jsonString + "\" , \"flowbatt\": \"" + testBit(m[19], 0) // low batt flag
		jsonString = jsonString + "\" , \"st\": \"" + sensor.SubType
		jsonString = jsonString + "\" }"

	case infosType7:
		log.Debug(", OREGON UV")
		log.Debug(", SubType=", binary.LittleEndian.Uint16(m[13:]))
		log.Debug(", idPHY=", binary.LittleEndian.Uint16(m[15:]))
		log.Debug(", idChannel=", binary.LittleEndian.Uint16(m[17:]))
		log.Debug(", qualifier=", binary.LittleEndian.Uint16(m[19:]))
		log.Debug(", light=", binary.LittleEndian.Uint16(m[21:]))

		sensor.Ref = "7-" + strconv.FormatUint(uint64(touint32(binary.LittleEndian.Uint16(m[15:]), binary.LittleEndian.Uint16(m[17:]))), 10)
		sensor.Protocol = "OREGON"
		sensor.SubType = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[13:])), 10)
		sensor.Name = sensorName(sensor.Ref)
		sensor.Topic = sensorTopic(sensor.Ref)
		if sensor.Topic == "NULL" {
			sensor.Topic = conf.GetString("brokermqtt.topicroot") + "/" + sensor.Ref + "/uv"
		}
		log.Debug(", topic=", sensor.Topic)

		lightString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[21:])), 10)

		topicSplit := strings.Split(sensor.Topic, "/")

		jsonString = "{ \"tc\": \"" + timecodeString
		jsonString = jsonString + "\" , \"n\": \"" + topicSplit[1]
		jsonString = jsonString + "\" , \"r\": \"" + sensor.Ref
		jsonString = jsonString + "\" , \"l\": \"" + lightString
		jsonString = jsonString + "\" , \"flowbatt\": \"" + testBit(m[19], 0) // low batt flag
		jsonString = jsonString + "\" , \"st\": \"" + sensor.SubType
		jsonString = jsonString + "\" }"

	case infosType8:
		log.Debug(", OWL")
		log.Debug(", SubType=", binary.LittleEndian.Uint16(m[13:]))
		log.Debug(", idPHY=", binary.LittleEndian.Uint16(m[15:]))
		log.Debug(", idChannel=", binary.LittleEndian.Uint16(m[17:]))
		log.Debug(", qualifier=", binary.LittleEndian.Uint16(m[19:]))
		log.Debug(", energy=", binary.LittleEndian.Uint32(m[21:]))
		log.Debug(", power=", binary.LittleEndian.Uint32(m[25:]))
		log.Debug(", powerI1=", binary.LittleEndian.Uint32(m[27:]))
		log.Debug(", powerI2=", binary.LittleEndian.Uint32(m[29:]))
		log.Debug(", powerI3=", binary.LittleEndian.Uint32(m[31:]))

		sensor.Ref = "8-" + strconv.FormatUint(uint64(touint32(binary.LittleEndian.Uint16(m[15:]), binary.LittleEndian.Uint16(m[17:]))), 10)
		sensor.Protocol = "OWL"
		sensor.SubType = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[13:])), 10)
		sensor.Name = sensorName(sensor.Ref)
		sensor.Topic = sensorTopic(sensor.Ref)
		if sensor.Topic == "NULL" {
			sensor.Topic = conf.GetString("brokermqtt.topicroot") + "/" + sensor.Ref + "/owl"
		}
		log.Debug(", topic=", sensor.Topic)

		energyString := strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[21:])), 10)
		powerString := strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[25:])), 10)
		powerI1String := strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[27:])), 10)
		powerI2String := strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[29:])), 10)
		powerI3String := strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[31:])), 10)

		topicSplit := strings.Split(sensor.Topic, "/")

		jsonString = "{ \"tc\": \"" + timecodeString
		jsonString = jsonString + "\" , \"n\": \"" + topicSplit[1]
		jsonString = jsonString + "\" , \"r\": \"" + sensor.Ref
		jsonString = jsonString + "\" , \"e\": \"" + energyString
		jsonString = jsonString + "\" , \"p\": \"" + powerString
		jsonString = jsonString + "\" , \"pi1\": \"" + powerI1String
		jsonString = jsonString + "\" , \"pi2\": \"" + powerI2String
		jsonString = jsonString + "\" , \"pi3\": \"" + powerI3String
		jsonString = jsonString + "\" , \"flowbatt\": \"" + testBit(m[19], 0) // low batt flag
		jsonString = jsonString + "\" , \"st\": \"" + sensor.SubType
		jsonString = jsonString + "\" }"

	case infosType9:
		log.Debug(", OREGON Rain")
		log.Debug(", SubType=", binary.LittleEndian.Uint16(m[13:]))
		log.Debug(", idPHY=", binary.LittleEndian.Uint16(m[15:]))
		log.Debug(", idChannel=", binary.LittleEndian.Uint16(m[17:]))
		log.Debug(", qualifier=", binary.LittleEndian.Uint16(m[19:]))
		log.Debug(", totalRain=", binary.LittleEndian.Uint32(m[21:]))
		log.Debug(", rain=", binary.LittleEndian.Uint16(m[25:]))

		sensor.Ref = "9-" + strconv.FormatUint(uint64(touint32(binary.LittleEndian.Uint16(m[15:]), binary.LittleEndian.Uint16(m[17:]))), 10)
		sensor.Protocol = "OREGON"
		sensor.SubType = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[13:])), 10)
		sensor.Name = sensorName(sensor.Ref)
		sensor.Topic = sensorTopic(sensor.Ref)
		if sensor.Topic == "NULL" {
			sensor.Topic = conf.GetString("brokermqtt.topicroot") + "/" + sensor.Ref + "/rain"
		}
		log.Debug(", topic=", sensor.Topic)

		totalrainString := strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[21:])), 10)
		rainString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[25:])), 10)

		topicSplit := strings.Split(sensor.Topic, "/")

		jsonString = "{ \"tc\": \"" + timecodeString
		jsonString = jsonString + "\" , \"n\": \"" + topicSplit[1]
		jsonString = jsonString + "\" , \"r\": \"" + sensor.Ref
		jsonString = jsonString + "\" , \"tra\": \"" + totalrainString
		jsonString = jsonString + "\" , \"ra\": \"" + rainString
		jsonString = jsonString + "\" , \"flowbatt\": \"" + testBit(m[19], 0) // low batt flag
		jsonString = jsonString + "\" , \"st\": \"" + sensor.SubType
		jsonString = jsonString + "\" }"

	case infosType10:
		log.Debug(", X2D Thermostat")
		log.Debug(", SubType=", binary.LittleEndian.Uint16(m[13:]))
		log.Debug(", id=", binary.LittleEndian.Uint32(m[15:]))
		log.Debug(", qualifier=", binary.LittleEndian.Uint16(m[19:]))
		log.Debug(", fonction=", binary.LittleEndian.Uint16(m[21:]))
		log.Debug(", mode=", binary.LittleEndian.Uint16(m[23:]))
		log.Debug(", data[4]=", binary.LittleEndian.Uint16(m[25:]))

		sensor.Ref = "10-" + strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[15:])), 10)
		sensor.Protocol = "X2D"
		sensor.SubType = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[13:])), 10)
		sensor.Name = sensorName(sensor.Ref)
		sensor.Topic = sensorTopic(sensor.Ref)
		if sensor.Topic == "NULL" {
			sensor.Topic = conf.GetString("brokermqtt.topicroot") + "/" + sensor.Ref + "/x2dcontact"
		}
		log.Debug(", topic=", sensor.Topic)

		qualifierString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[19:])), 10)

		topicSplit := strings.Split(sensor.Topic, "/")

		jsonString = "{ \"tc\": \"" + timecodeString
		jsonString = jsonString + "\" , \"n\": \"" + topicSplit[1]
		jsonString = jsonString + "\" , \"r\": \"" + sensor.Ref
		jsonString = jsonString + "\" , \"q\": \"" + qualifierString
		jsonString = jsonString + "\" , \"ftamper\": \"" + testBit(m[19], 0)    // tamper flag
		jsonString = jsonString + "\" , \"fanomaly\": \"" + testBit(m[19], 1)   // anomaly flag
		jsonString = jsonString + "\" , \"flowbatt\": \"" + testBit(m[19], 2)   // low batt flag
		jsonString = jsonString + "\" , \"ftestassoc\": \"" + testBit(m[19], 4) // test assoc flag
		jsonString = jsonString + "\" , \"fdomestic\": \"" + testBit(m[19], 5)  // domestic frame flag
		jsonString = jsonString + "\" , \"st\": \"" + sensor.SubType
		jsonString = jsonString + "\" }"

	case infosType11:
		log.Debug(", X2D Shutter")
		log.Debug(", SubType=", binary.LittleEndian.Uint16(m[13:]))
		log.Debug(", id=", binary.LittleEndian.Uint32(m[15:]))
		log.Debug(", qualifier=", binary.LittleEndian.Uint16(m[19:]))
		log.Debug(", reserved1=", binary.LittleEndian.Uint16(m[21:]))
		log.Debug(", reserved2=", binary.LittleEndian.Uint16(m[23:]))
		log.Debug(", data[4]=", binary.LittleEndian.Uint16(m[25:]))

		sensor.Ref = "11-" + strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[15:])), 10)
		sensor.Protocol = "X2D"
		sensor.SubType = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[13:])), 10)
		sensor.Name = sensorName(sensor.Ref)
		sensor.Topic = sensorTopic(sensor.Ref)
		if sensor.Topic == "NULL" {
			sensor.Topic = conf.GetString("brokermqtt.topicroot") + "/" + sensor.Ref + "/x2dshutter"
		}
		log.Debug(", topic=", sensor.Topic)

		qualifierString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[19:])), 10)

		log.Debug(", topic=", sensor.Topic)

		topicSplit := strings.Split(sensor.Topic, "/")

		jsonString = "{ \"tc\": \"" + timecodeString
		jsonString = jsonString + "\" , \"n\": \"" + topicSplit[1]
		jsonString = jsonString + "\" , \"r\": \"" + sensor.Ref
		jsonString = jsonString + "\" , \"q\": \"" + qualifierString
		jsonString = jsonString + "\" , \"ftamper\": \"" + testBit(m[19], 0)    // tamper flag
		jsonString = jsonString + "\" , \"fanomaly\": \"" + testBit(m[19], 1)   // anomaly flag
		jsonString = jsonString + "\" , \"flowbatt\": \"" + testBit(m[19], 2)   // low batt flag
		jsonString = jsonString + "\" , \"ftestassoc\": \"" + testBit(m[19], 4) // test assoc flag
		jsonString = jsonString + "\" , \"fdomestic\": \"" + testBit(m[19], 5)  // domestic frame flag
		jsonString = jsonString + "\" , \"st\": \"" + sensor.SubType
		jsonString = jsonString + "\" }"

	case infosType12:
		log.Debug(", deprecated")
		log.Debug(", SubType=", binary.LittleEndian.Uint16(m[13:]))
		log.Debug(", id=", binary.LittleEndian.Uint32(m[15:]))
		log.Debug(", qualifier=", binary.LittleEndian.Uint16(m[19:]))
		log.Debug(", temp=", (int16)(binary.LittleEndian.Uint16(m[21:])))
		log.Debug(", setPoint=", (int16)(binary.LittleEndian.Uint16(m[23:])))

		sensor.Ref = "12-" + strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[15:])), 10)
		sensor.Protocol = "DEPRECATED"
		sensor.SubType = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[13:])), 10)
		sensor.Name = sensorName(sensor.Ref)
		sensor.Topic = sensorTopic(sensor.Ref)
		if sensor.Topic == "NULL" {
			sensor.Topic = conf.GetString("brokermqtt.topicroot") + "/" + sensor.Ref + "/null"
		}
		log.Debug(", topic=", sensor.Topic)

		qualifierString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[19:])), 10)

		topicSplit := strings.Split(sensor.Topic, "/")

		jsonString = "{ \"tc\": \"" + timecodeString
		jsonString = jsonString + "\" , \"n\": \"" + topicSplit[1]
		jsonString = jsonString + "\" , \"r\": \"" + sensor.Ref
		jsonString = jsonString + "\" , \"q\": \"" + qualifierString
		jsonString = jsonString + "\" , \"st\": \"" + sensor.SubType
		jsonString = jsonString + "\" }"

	case infosType13:
		log.Debug(", Linky")
		log.Debug(", SubType=", binary.LittleEndian.Uint16(m[13:]))
		log.Debug(", id=", binary.LittleEndian.Uint32(m[15:]))
		log.Debug(", qualifier=", binary.LittleEndian.Uint16(m[19:]))
		log.Debug(", contractType=", binary.LittleEndian.Uint16(m[21:]))
		log.Debug(", setPoint=", (int16)(binary.LittleEndian.Uint16(m[23:])))
		log.Debug(", cnt1=", binary.LittleEndian.Uint32(m[25:]))
		log.Debug(", cnt2=", binary.LittleEndian.Uint32(m[29:]))
		log.Debug(", apparentPower=", binary.LittleEndian.Uint16(m[33:]))

		sensor.Ref = "13-" + strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[15:])), 10)
		sensor.Protocol = "LINKY"
		sensor.SubType = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[13:])), 10)
		sensor.Name = sensorName(sensor.Ref)
		sensor.Topic = sensorTopic(sensor.Ref)
		if sensor.Topic == "NULL" {
			sensor.Topic = conf.GetString("brokermqtt.topicroot") + "/" + sensor.Ref + "/linky"
		}
		log.Debug(", topic=", sensor.Topic)

		contracttypeString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[21:])), 10)
		setpointString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[23:])), 10)
		cnt1String := strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[25:])), 10)
		cnt2String := strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[29:])), 10)
		apparentpowerString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[33:])), 10)
		qualifierString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[19:])), 10)

		topicSplit := strings.Split(sensor.Topic, "/")

		jsonString = "{ \"tc\": \"" + timecodeString
		jsonString = jsonString + "\" , \"n\": \"" + topicSplit[1]
		jsonString = jsonString + "\" , \"r\": \"" + sensor.Ref
		jsonString = jsonString + "\" , \"ct\": \"" + contracttypeString
		jsonString = jsonString + "\" , \"sp\": \"" + setpointString
		jsonString = jsonString + "\" , \"cnt1\": \"" + cnt1String
		jsonString = jsonString + "\" , \"cnt2\": \"" + cnt2String
		jsonString = jsonString + "\" , \"ap\": \"" + apparentpowerString
		jsonString = jsonString + "\" , \"q\": \"" + qualifierString
		jsonString = jsonString + "\" , \"st\": \"" + sensor.SubType
		jsonString = jsonString + "\" }"

	case infosType14:
		log.Debug(", FS20")
		log.Debug(", SubType=", binary.LittleEndian.Uint16(m[13:]))
		log.Debug(", id=", binary.LittleEndian.Uint32(m[15:]))
		log.Debug(", qualifier=", binary.LittleEndian.Uint16(m[19:]))

		sensor.Ref = "14-" + strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[15:])), 10)
		sensor.Protocol = "FS20"
		sensor.SubType = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[13:])), 10)
		sensor.Name = sensorName(sensor.Ref)
		sensor.Topic = sensorTopic(sensor.Ref)
		if sensor.Topic == "NULL" {
			sensor.Topic = conf.GetString("brokermqtt.topicroot") + "/" + sensor.Ref + "/fs20"
		}
		log.Debug(", topic=", sensor.Topic)

		qualifierString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[19:])), 10)

		topicSplit := strings.Split(sensor.Topic, "/")

		jsonString = "{ \"tc\": \"" + timecodeString
		jsonString = jsonString + "\" , \"n\": \"" + topicSplit[1]
		jsonString = jsonString + "\" , \"r\": \"" + sensor.Ref
		jsonString = jsonString + "\" , \"q\": \"" + qualifierString
		jsonString = jsonString + "\" , \"st\": \"" + sensor.SubType
		jsonString = jsonString + "\" }"

	case infosType15:
		log.Debug(", JAMMING")
		log.Debug(", SubType=", binary.LittleEndian.Uint16(m[13:]))
		log.Debug(", Id=", binary.LittleEndian.Uint32(m[15:]))

		sensor.Ref = "15-" + strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[15:])), 10)
		sensor.Protocol = "JAMMING"
		sensor.SubType = strconv.FormatUint(uint64(binary.LittleEndian.Uint32(m[13:])), 10)
		sensor.Name = sensorName(sensor.Ref)
		sensor.Topic = sensorTopic(sensor.Ref)
		if sensor.Topic == "NULL" {
			sensor.Topic = conf.GetString("brokermqtt.topicroot") + "/" + sensor.Ref + "/jamming"
		}
		log.Debug(", topic=", sensor.Topic)

		subtypeString := strconv.FormatUint(uint64(binary.LittleEndian.Uint16(m[13:])), 10)

		topicSplit := strings.Split(sensor.Topic, "/")

		jsonString = "{ \"tc\": \"" + timecodeString
		jsonString = jsonString + "\" , \"n\": \"" + topicSplit[1]
		jsonString = jsonString + "\" , \"r\": \"" + sensor.Ref
		jsonString = jsonString + "\" , \"s\": \"" + subtypeString
		jsonString = jsonString + "\" , \"st\": \"" + sensor.SubType
		jsonString = jsonString + "\" }"

	}

	/**
	 * Send the MQTT message in non blocking way
	 */
	log.Debug("Publication MQTT jsonString : ", jsonString)
	go publish(sensor.Topic, jsonString)
}

/**
 * Function the publish a MQTT message with topic t and message d
 */
func publish(t string, d string) {
	var token mqtt.Token

	if cmqtt.IsConnectionOpen() {
		token = cmqtt.Publish(t, 2, false, d)
		token.Wait()
	}
}

/**
 * Function that send a byte array to the serial port of RFPLayer module
 */
func emit(p io.ReadWriteCloser) {
	var n int
	var err error

	/**
	 * Loop to process sequentially each message received
	 */
	for {
		/**
		 * Send the message in the buffered channel
		 */
		log.Debug(time.Now(), " : wait for message")
		n, err = p.Write(<-ch)
		if err != nil {
			if err != io.EOF {
				log.Error("Error writing to serial port: ", err)
			}
		} else {
			log.Debug(time.Now(), " : ", n, " bytes wrote")
		}

		/**
		 * Sleep iWait2Send not to block rfp1000 dongle
		 */
		time.Sleep(time.Duration(iWait2Send) * time.Millisecond)
	}
}

/**
 * Function that handle a stream of bytes from RFPlayer dongle
 */
func receive(p io.ReadWriteCloser) {

	spool := new(bytes.Buffer)
	lspool := 0

	for {
		buf := make([]byte, 1024) // Byte array to receive from serial port
		n, err := p.Read(buf)     // Read from serial port
		if err != nil {
			if err != io.EOF {
				log.Error("++++++> Error reading from serial port: ", err)
			}
		}

		/**
		 * Append to spool
		 */
		l, err := spool.Write(buf[:n])
		if err != nil {
			log.Error("++++++> Error saving to spool of length ", l, " : ", err)
		}

		/**
		 * Update the spool length
		 */
		lspool = lspool + n

		/**
		 * Transfer in a byte array
		 */
		spoolbytes := spool.Bytes()

		/**
		 * Look for 'ZI' which is start of message
		 */
		i := bytes.Index(spoolbytes, []byte("ZI"))

		/**
		 * If 'ZI' found
		 */
		if i != -1 {
			/**
			 * Is there enough bytes to compute the payload length
			 */
			if i+4 < lspool {
				/**
				 * Display source-dest value
				 */
				//log.Info("SourceDest value : ", (int)(spoolbytes[i+2]))

				/**
				 * Extract the length of payload
				 */
				payloadlen := (int)(spoolbytes[i+3]) + ((int)(spoolbytes[i+4]) * 256)

				/**
				 * Message complete in spool ?
				 */
				if payloadlen+5 <= lspool {
					/**
					 * Discard unusefull bytes at beginning
					 */
					spool.Next(i)
					lspool = lspool - i

					/**
					 * Extract message from spool
					 */
					message := spool.Next(payloadlen + 5)
					lspool = lspool - (payloadlen + 5)

					/**
					 * Send to decode
					 */
					log.Debug("Message to decode -->", string(message[:payloadlen+5]), "<-- ")
					decode(payloadlen+5, message[:payloadlen+5])
				}
			}
		} else {
			/**
			 * Start message not found. If length in spool bigger than 64 bytes, discard spool and log it in Error
			 * As a message should be 28 bytes long maximum, we get an extra space before discarding
			 */
			spool.Next(lspool)
			lspool = 0
			log.Error("++++++> Error, no 'ZI' found in the spool buffer")
		}
	}
}

/**
 * Function that handle MQTT message related to "subscribe"
 *
 * - The commands passed by MQTT messages : home/action/...
 *   as exemple to close shutter : home/action/<nom_volet> payload down
 * 			       appairing a DIO plug : home/action/<plug_name> payload assoc
 *				   set on DIO plug : home/action/<plug_name> payload on
 *				   set off a DIO plug : home/action/<plug_name> payload off
 */
var fMqttMsgHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	b := new(bytes.Buffer)

	log.Debug(time.Now(), " --- fMqttMsgHandler TOPIC: ", msg.Topic(), " MSG: ", msg.Payload(), " - l : ", cap(msg.Payload()))

	/**
	 * Split on char /
	 */
	topicSplit := strings.Split(string(msg.Topic()), "/")

	/**
	 * Deal with a command if topic is like home/action
	 */
	if topicSplit[0] == "home" && topicSplit[1] == "action" && len(topicSplit[2]) > 0 {

		/**
		 * Add header
		 */
		b.Write([]byte("ZI")) // ZI

		/**
		 * Add sourdest value, \x01 for 433/868
		 */
		b.Write([]byte("\x01"))

		/**
		 * Add the length of the message (12 for the moment)
		 */
		b.Write([]byte("\x0C\x00"))

		/**
		 * Add binary data
		 */
		b.Write([]byte("\x00")) // FrameType
		b.Write([]byte("\x00")) // Cluster

		/**
		 * Configure the protocol variable from the conf of the actuator and add it to buffer
		 */
		switch actuatorProtocol(topicSplit[2]) {
		case "visonic433":
			b.Write([]byte("\x01"))
		case "visonic868":
			b.Write([]byte("\x02"))
		case "chacon", "dio":
			b.Write([]byte("\x03"))
		case "domia":
			b.Write([]byte("\x04"))
		case "x10":
			b.Write([]byte("\x05"))
		case "x2d433":
			b.Write([]byte("\x06"))
		case "x2d868":
			b.Write([]byte("\x07"))
		case "x2dshutter":
			b.Write([]byte("\x08"))
		case "x2dhaelec":
			b.Write([]byte("\x09"))
		case "x2dhagas":
			b.Write([]byte("\x0A"))
		case "somfyrts", "rts":
			b.Write([]byte("\x0B"))
		case "blyss":
			b.Write([]byte("\x0C"))
		case "parrot":
			b.Write([]byte("\x0D"))
		case "fs20":
			b.Write([]byte("\x0E"))
		case "kd101":
			b.Write([]byte("\x0F"))
		case "edisio":
			b.Write([]byte("\x10"))
		}

		switch actuatorProtocol(topicSplit[2]) {
		case "visonic433", "visonic868", "chacon", "dio", "domia", "x10", "x2d433", "x2d868", "x2dshutter", "x2dhagas", "somfyrts", "rts", "blyss", "parrot", "fs20", "kd101", "edisio":
			switch string(msg.Payload()) {
			case "0": // OFF
				b.Write([]byte("\x00"))
			case "1": // ON
				b.Write([]byte("\x01"))
			case "2": // DIM
				b.Write([]byte("\x02"))
			case "6": // ASSOC
				b.Write([]byte("\x06"))
			default:
				log.Debug(time.Now(), " --- fMqttMsgHandler : Unknown payload : ", string(msg.Payload()))
			}
		case "x2dhaelec":
			log.Debug(time.Now(), " --- fMqttMsgHandler : in X2DHAELEC with payload : ", string(msg.Payload()))
			switch string(msg.Payload()) {
			case "AutoLow", "EcoLow", "ConfortLow": // => OFF
				b.Write([]byte("\x00"))
			case "Auto", "Eco", "Confort", "Stop", "HorsGel": // => ON
				b.Write([]byte("\x01"))
			default:
				log.Debug(time.Now(), " --- fMqttMsgHandler : Unknown payload : ", string(msg.Payload()))
			}
		default:
			log.Debug(time.Now(), " --- fMqttMsgHandler : unknown protocol ", actuatorProtocol(topicSplit[2]))
		}

		/**
		 * Get the code with the name of the actuator
		 */
		h := atobDeviceID(actuatorID(topicSplit[2])) // DeviceID 4 bytes LSB First
		a := make([]byte, 4)
		binary.LittleEndian.PutUint32(a, h)
		b.Write(a)

		switch actuatorProtocol(topicSplit[2]) {
		case "visonic433", "dio", "chacon":
			b.Write([]byte("\x00")) // DimValue 0% to 100%
		case "somfyrts", "rts":
			if string(msg.Payload()) != "2" {
				b.Write([]byte("\x00")) // DimValue 0% to 100%
			} else {
				b.Write([]byte("\x04")) // DimValue 4% if RTS to emulate My function
			}
		case "x2dhaelec":
			switch string(msg.Payload()) {
			case "Eco", "EcoLow": // => %0
				b.Write([]byte("\x00")) // Action 0 : OFF / 1 : ON
			case "Confort", "ConfortLow": // => %3
				b.Write([]byte("\x03")) // Action 0 : OFF / 1 : ON
			case "Stop": // => %4
				b.Write([]byte("\x04")) // Action 0 : OFF / 1 : ON
			case "HorsGel": // => %5
				b.Write([]byte("\x05")) // Action 0 : OFF / 1 : ON
			case "Auto", "AutoLow": // => %7
				b.Write([]byte("\x07")) // Action 0 : OFF / 1 : ON
			}
		}

		b.Write([]byte("\x00")) // Burst 0 by default
		b.Write([]byte("\x00")) // Qualifier 0 by default
		b.Write([]byte("\x00")) // Reserved2 0 by default

		if conf.GetString("log.level") == "debug" {
			dumpByteSlice(b.Bytes())
		}

		/**
		 * Send the bytes array to the channel
		 */
		ch <- b.Bytes()
	}
}

/**
 * Build cache array from the sensors data in the config file
 */
func loadSensors() {

	/**
	 * What is the number of sensors
	 */
	log.Info("Number of sensors added : ", len(config.Sensors))

	/**
	 * Build the cache
	 */
	sensorsNameCache = cache.New(cache.NoExpiration, cache.NoExpiration)
	sensorsTopicCache = cache.New(cache.NoExpiration, cache.NoExpiration)

	/**
	 * Load the cache
	 */
	for i := 0; i < len(config.Sensors); i++ {
		id := config.Sensors[i].ID
		name := config.Sensors[i].Name
		topic := config.Sensors[i].Topic
		ref := config.Sensors[i].Ref
		log.Info("Loading sensor data ", i, " Id:", id, " Name:", name, " Topic:", topic, " Ref:", ref)

		/**
		 * Name cache
		 */
		err := sensorsNameCache.Add(id, name, cache.NoExpiration)
		if err != nil {
			log.Info("ERROR while adding sensor in name cache, already defined ", id, " !!!")
		}

		/**
		 * Topic cache
		 */
		if topic != "" {
			err := sensorsTopicCache.Add(id, topic, cache.NoExpiration)
			if err != nil {
				log.Info("ERROR while adding sensor in topic cache, already defined ", id, " !!!")
			}
		} else {
			/**
			 * Si pas de topic défini, on prend le paramètre name
			 */
			err := sensorsTopicCache.Add(id, name, cache.NoExpiration)
			if err != nil {
				log.Info("ERROR while adding sensor in topic cache, already defined ", id, " !!!")
			}

		}

		log.Info("[loadSensors] Number of sensors defined : ", sensorsNameCache.ItemCount())
	}
}

/**
 * Build cache array from the actuators data in the config file
 */
func loadActuators() {

	/**
	 * What is the number of actuators
	 */
	log.Info("Number of actuators added : ", len(config.Actuators))

	/**
	 * Build the cache
	 */
	actuatorsIDCache = cache.New(cache.NoExpiration, cache.NoExpiration)
	actuatorsTopicCache = cache.New(cache.NoExpiration, cache.NoExpiration)
	actuatorsCommandCache = cache.New(cache.NoExpiration, cache.NoExpiration)
	actuatorsProtocolCache = cache.New(cache.NoExpiration, cache.NoExpiration)

	/**
	 * Load the cache
	 */
	for i := 0; i < len(config.Actuators); i++ {
		name := config.Actuators[i].Name

		/**
		 * Name cache
		 */
		value := config.Actuators[i].ID
		log.Info("Loading actuator data ", i, " Name:", name, " Id:", value)
		err := actuatorsIDCache.Add(name, value, cache.NoExpiration)
		if err != nil {
			log.Info("ERROR while adding actuator name, already defined ", name, " !!!")
		}

		/**
		 * Topics cache
		 */
		value = config.Actuators[i].Topic
		log.Info("Loading actuator data ", i, " Name:", name, " Topic:", value)
		err = actuatorsTopicCache.Add(name, value, cache.NoExpiration)
		if err != nil {
			log.Info("ERROR while adding actuator topic, already defined ", name, " !!!")
		}

		/**
		 * Command cache
		 */
		value = config.Actuators[i].Command
		log.Info("Loading actuator command ", i, " Name:", name, " Command:", value)
		err = actuatorsCommandCache.Add(name, value, cache.NoExpiration)
		if err != nil {
			log.Info("ERROR while adding actuator command, already defined ", name, " !!!")
		}

		/**
		 * Protocol cache
		 */
		value = config.Actuators[i].Protocol
		log.Info("Loading actuator command protocol ", i, " Name:", name, " Protocol:", value)
		err = actuatorsProtocolCache.Add(name, value, cache.NoExpiration)
		if err != nil {
			log.Info("ERROR while adding actuator protocol, already defined ", name, " !!!")
		}
	}

	log.Info("[loadActuators] Numbre of actuator defined : ", actuatorsIDCache.ItemCount())
}

/**
 * Function that return the sensor name by its ID
 */
func sensorName(sensorID string) string {
	var r string

	foo, found := sensorsNameCache.Get(sensorID)
	if found {
		r = foo.(string)
	} else {
		r = "NULL"
	}

	return r
}

/**
 * Function that return the sensor topic by its ID if it exists
 */
func sensorTopic(sensorID string) string {
	var r string

	foo, found := sensorsTopicCache.Get(sensorID)
	if found {
		r = foo.(string)
	} else {
		r = "NULL"
		log.Info("Fail to find sensorId : >", sensorID, "<")
	}

	return r
}

/**
 * Function the return a X10 code of the actuator by its name
 */
func actuatorID(actuatorName string) string {
	var r string

	foo, found := actuatorsIDCache.Get(actuatorName)
	if found {
		r = foo.(string)
	} else {
		r = "NULL"
	}

	log.Debug(time.Now(), " ### actuatorID of ", actuatorName, " is >", r, "<")

	return r
}

/**
 * Function that return the protocol of the actuator by its name
 */
func actuatorProtocol(actuatorName string) string {
	var r string

	foo, found := actuatorsProtocolCache.Get(actuatorName)
	if found {
		r = foo.(string)
	} else {
		r = "NULL"
	}

	return r
}

/**
 * Function called when the MQTT connection is UP
 *
 */
func connUpHandler(c mqtt.Client) {
	log.Info("[MQTT] Connection up...")

	// Subscribe now we are connected
	if tokenS := cmqtt.Subscribe("home/action/#", 2, fMqttMsgHandler); tokenS.Wait() && tokenS.Error() != nil {
		log.Info("[MQTT] Subscription failed...")
		//panic(tokenS.Error())
	} else {
		log.Info("[MQTT] Subscribed to home/action/# topic ...")
	}
}

/**
 * Function called when the MQTT connection is lost
 *
 */
func connLostHandler(c mqtt.Client, err error) {
	log.Info("[MQTT] Connection lost, reason: ", err)

	//Perform additional action...
}

/**
 * Function called to setup MQTT client and connect
 *
 */
func mqttSetupAndConnect() {
	/**
	 * Setup MQTT
	 */
	var broker bytes.Buffer
	broker.WriteString(conf.GetString("brockermqtt.protocol"))
	broker.WriteString("://")
	broker.WriteString(conf.GetString("brockermqtt.address"))
	broker.WriteString(":")
	broker.WriteString(conf.GetString("brockermqtt.port"))

	log.Info("[MQTT] connection URL : ", broker.String())

	cmqttOpts := mqtt.NewClientOptions()

	if conf.GetString("brockermqtt.protocol") == "tls" {
		// TLS connexion
		insecure := conf.GetBool("brockermqtt.insecure")
		if insecure {
			log.Info("[MQTT] Insecure SSL, does not verify certificate...")
		}

		// Get the SystemCertPool, continue with an empty pool on error
		rootCAs, _ := x509.SystemCertPool()
		if rootCAs == nil {
			log.Info("[MQTT] No rootCAs, get an empty one...")
			rootCAs = x509.NewCertPool()
		}

		// Read in the cert file
		certs, err := ioutil.ReadFile(conf.GetString("brockermqtt.certfile"))
		if err != nil {
			log.Fatal("[MQTT] Failed to append ", conf.GetString("brockermqtt.certfile"), " to RootCAs: ", err)
		}

		// Append our cert to the system pool
		if ok := rootCAs.AppendCertsFromPEM(certs); !ok {
			log.Info("[MQTT] No certs appended, using system certs only")
		}

		// Trust the augmented cert pool in our client
		tlsConfig := &tls.Config{
			RootCAs:            rootCAs,
			InsecureSkipVerify: insecure,
		}

		cmqttOpts.SetTLSConfig(tlsConfig) //we set the tls configuration
	}

	cmqttOpts.AddBroker(broker.String())                          // Add broker information
	cmqttOpts.SetClientID("rfp2mqtt_pubsub")                      // Add client_id
	cmqttOpts.SetUsername(conf.GetString("brockermqtt.username")) // Add username
	cmqttOpts.SetPassword(conf.GetString("brockermqtt.password")) // And password
	cmqttOpts.SetConnectionLostHandler(connLostHandler)           // Add also en handler for handling lost connection
	cmqttOpts.SetOnConnectHandler(connUpHandler)                  // Add hendler when connection is performed
	cmqttOpts.AutoReconnect = false

	cmqtt = mqtt.NewClient(cmqttOpts)
	if tokenC := cmqtt.Connect(); tokenC.Wait() && tokenC.Error() != nil {
		log.Info("[MQTT] Connection failed...")
		// panic(tokenC.Error())
	} else {
		log.Info("[MQTT] Connected to broker...")
	}
}

/**
 * Function called when rfplayer start
 *
 * - Read configuration
 * - Configure the logs
 */
func init() {
	/**
	 * Setup default conf
	 */
	// Configuration par défaut
	conf.SetDefault("rfplayer.waittosend", "500")            // Time between to message send to rfp module
	conf.SetDefault("rfplayer.port", "/dev/ttyUSB0")         // Port série
	conf.SetDefault("rfplayer.baud", "115200")               // Baud rate
	conf.SetDefault("rfplayer.data", "8")                    // Data bits
	conf.SetDefault("rfplayer.parity", "none")               // Parity : none / odd / even
	conf.SetDefault("rfplayer.stop", "1")                    // Stop bits
	conf.SetDefault("rfplayer.rtsctsflowcontrol", "false")   // RTSCTS flow control
	conf.SetDefault("rfplayer.rs485", "false")               // enable RS485 RTS for direction control
	conf.SetDefault("rfplayer.rs485highduringsend", "false") // RTS signal should be high during send
	conf.SetDefault("rfplayer.rs485highaftersend", "false")  // RTS signal should be high after send
	conf.SetDefault("rfplayer.timeout", "100")               // Inter Character timeout (ms)
	conf.SetDefault("rfplayer.minread", "10")                // Minimum read count
	conf.SetDefault("rfplayer.rx", "true")                   // Activate Read data Received
	conf.SetDefault("rfplayer.jamming", "10")                // Level of Jamming
	conf.SetDefault("brokermqtt.protocol", "tls")
	conf.SetDefault("brokermqtt.address", "127.0.0.1")
	conf.SetDefault("brockermqtt.port", "1883")
	conf.SetDefault("brockermqtt.username", "username")
	conf.SetDefault("brockermqtt.password", "password")
	conf.SetDefault("brockermqtt.certfile", "ca.crt")
	conf.SetDefault("brockermqtt.insecure", "false")
	conf.SetDefault("brockermqtt.topicroot", "rfp2mqtt")
	conf.SetDefault("log.format", "ascii")
	conf.SetDefault("log.output", "stdout")
	conf.SetDefault("log.level", "info")

	/**
	 * Initialize config parameters passed by command line if present
	 */
	flag.StringVar(&flagConfigFile, "c", "UNDEFINED", "Location and name of config file")
	// insecure = flag.Bool("insecure-ssl", false, "Accept/Ignore all server SSL certificates")
	flag.Parse()
	log.Info("[init] config file which will be used : ", flagConfigFile)

	/**
	 * Load config file
	 */
	if flagConfigFile != "UNDEFINED" {
		// Load config data from file specified by command line
		conf.SetConfigFile(flagConfigFile) // Configfile location and name
	} else {
		// Load config data from file defined by default in code
		conf.SetConfigName("config") // config name without extension
		conf.AddConfigPath(".")      // option where to read the config file
		conf.AddConfigPath("/dist")  // option where to read the config file
		//conf.AddConfigPath("/home/jean-yves/go/src/rfp2mqtt") // option where to read the config file
	}
	err := conf.ReadInConfig() // Read the config file
	if err != nil {            // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s", err))
	}

	/**
	 * Filling up configuration struct
	 */
	errnew := conf.Unmarshal(&config)
	if errnew != nil {
		panic("Unable to unmarshal config")
	}

	/**
	 * Loading of sensors and actuators in memory
	 */
	loadSensors()
	loadActuators()

	if config.Log.Format == "json" {
		log.SetFormatter(&log.JSONFormatter{})
	}

	switch config.Log.Output {
	case "stdout":
		// Output to stdout instead of the default stderr
		// Can be any io.Writer, see below for File example
		log.SetOutput(os.Stdout)
	case "stderr":
		// Output to stdout instead of the default stderr
		// Can be any io.Writer, see below for File example
		log.SetOutput(os.Stderr)
	default:
		// Output to stdout instead of the default stderr
		// Can be any io.Writer, see below for File example
		log.SetOutput(os.Stdout)
	}

	logLevel, logerr := log.ParseLevel(config.Log.Level)
	if logerr != nil {
		panic(fmt.Errorf("Fatal error parsing level: %s", logerr))
	}

	log.SetLevel(logLevel)
}

func dumpByteSlice(b []byte) {
	var a [16]byte
	n := (len(b) + 15) &^ 15
	for i := 0; i < n; i++ {
		if i%16 == 0 {
			fmt.Printf("%4d", i)
		}
		if i%8 == 0 {
			fmt.Print(" ")
		}
		if i < len(b) {
			fmt.Printf(" %02X", b[i])
		} else {
			fmt.Print("   ")
		}
		if i >= len(b) {
			a[i%16] = ' '
		} else if b[i] < 32 || b[i] > 126 {
			a[i%16] = '.'
		} else {
			a[i%16] = b[i]
		}
		if i%16 == 15 {
			fmt.Printf("  %s\n", string(a[:]))
		}
	}
}

func main() {

	var err error
	var bparity rfp.ParityMode

	/**
	 * Serial configuration with RFPLAYER dongle
	 */
	switch conf.GetString("rfplayer.parity") {
	case "none":
		bparity = rfp.PARITY_NONE
	case "odd":
		bparity = rfp.PARITY_ODD
	case "even":
		bparity = rfp.PARITY_EVEN
	}

	options := rfp.OpenOptions{
		PortName:               conf.GetString("rfplayer.port"),
		BaudRate:               uint(conf.GetInt("rfplayer.baud")),
		DataBits:               uint(conf.GetInt("rfplayer.data")),
		StopBits:               uint(conf.GetInt("rfplayer.stop")),
		MinimumReadSize:        uint(conf.GetInt("rfplayer.minread")),
		InterCharacterTimeout:  uint(conf.GetInt("rfplayer.timeout")),
		ParityMode:             bparity,
		RTSCTSFlowControl:      conf.GetBool("rfplayer.rtsctsflowcontrol"),
		Rs485Enable:            conf.GetBool("rfplayer.rs485"),
		Rs485RtsHighDuringSend: conf.GetBool("rfplayer.rs485highduringsend"),
		Rs485RtsHighAfterSend:  conf.GetBool("rfplayer.rs485highaftersend"),
	}
	log.Info("MinimumReadSize ", uint(conf.GetInt("rfplayer.minread")))
	log.Info("InterCharacterTimeout ", uint(conf.GetInt("rfplayer.timeout")))
	log.Info("RTSCTSFlowControl ", conf.GetBool("rfplayer.rtsctsflowcontrol"))

	rfpPort, err = rfp.Open(options)

	if err != nil {
		log.Error("Error opening serial port ", conf.GetString("rfplayer.port"), " : ", err)
		os.Exit(-1)
	} else {
		log.Info("Connection done to RFPlayer dongle on port ", conf.GetString("rfplayer.port"))
		defer rfpPort.Close()
	}

	/**
	 * Default configuration of RFPLAYER dongle by sending command in config.yml
	 */
	tData := []byte("")
	for i := 0; i < len(config.Rfplayer.Initialisation); i++ {
		tData = []byte(config.Rfplayer.Initialisation[i].Cmd + "\x00")
		count, err := rfpPort.Write(tData)
		if err != nil {
			log.Error("Error writing to serial port: ", err)
		} else {
			log.Debug("Wrote ", count, " bytes : ", string(tData[:]))
		}
	}

	/**
	 * Setup time between 2 send message to RFP
	 */
	iWait2Send = config.Rfplayer.WaitToSend

	/**
	 * Openning reception
	 */
	if conf.GetBool("rfplayer.rx") {
		log.Info("Openning reception...")
		go receive(rfpPort)
	}

	/**
	 * Create the channel for incoming messages
	 */
	ch = make(chan []byte, 100)

	/**
	 * Launch the emit process
	 */
	go emit(rfpPort)

	/**
	 * Setup MQTT
	 */
	mqttSetupAndConnect()

	/**
	 * Sending a watchdog message every 10 seconds
	 * check if connected, if not, try reconnecting
	 */
	for {
		time.Sleep(10 * time.Second)
		if cmqtt.IsConnectionOpen() {
			go publish("rfplayer/watchdog", time.Now().Format(time.RFC3339))
		} else {
			// Try reconnecting
			mqttSetupAndConnect()
		}
	}
}
