

## Résumé

Passerelle entre le dongle RFPlayer (RFP1000 - https://rfplayer.com/) et un broker MQTT

## Objectif

Ce projet permet d'utiliser un dongle RFPlayer dans une installation domotique qui dispose d'un broker MQTT

Ce composant fait partie d'un système global ayant pour but de se substituer à une solution Zibase et d'y intégrer par la même occasion des éléments indépendants.

Il permet ainsi l'intégration quasi immédiate des capteurs Visonic ainsi que des sondes Oregon utilsés sur la Zibase (hors des périphériques ZWave).

## Architecture

rfp2mqtt est composé simplement d'un exécutable écrit en Go et d'un fichier de configuration (config.yml par défaut)

## Gestion de la configuration

Le fichier de configuration est un fichier YAML.

Il est composé de plusieurs sections

### Section RFPlayer

```
    waittosend: 500 			// When sending out radio messages, delay in ms to wait between 2 messages
    port: /dev/ttyUSB0 			// Port on which the RFPlayer is connected
    baud: 115200 				// Baud rate
    data: 8 					// Data bits
    parity: none				// Parity : none / odd / even
    stop: 1 					// Stop bits
    timeout: 100 				// Inter character timeout (ms)
    minread: 4 					// Minimum read count
    rtsctsflowcontrol: true 	// Flowcontrol
    rs485:false					// enable RS485 RTS for direction control
    rs485highduringsend: false	// RTS signal should be high during send
    rs485highaftersend: false	// RTS signal should be high after send
    rx: true					// Activate Read data Received
    initialisation: 			// Command to initialize the RFPlayer
        -
            cmd: 'ZIA++REPEATER OFF'
        -
            cmd: 'ZIA++RFLINK 0'
        -
            cmd: 'ZIA++LEDACTIVITY 1'
        -
            cmd: 'ZIA++FREQ L 433920'
        -
            cmd: 'ZIA++SELECTIVITY L 0'
        -
            cmd: 'ZIA++DSPTRIGGER L 8'
        -
            cmd: 'ZIA++FREQ H 868950'
        -
            cmd: 'ZIA++SELECTIVITY H 0'
        -
            cmd: 'ZIA++DSPTRIGGER H 6'
        -
            cmd: 'ZIA++LBT 16'
        -
            cmd: 'ZIA++FORMAT RFLINK OFF'
        -
            cmd: 'ZIA++FORMAT BINARY'
        -
            cmd: 'ZIA++TRACE - *'
        -
            cmd: 'ZIA++JAMMING 7'


```

### Section BrockerMQTT

```
    username: xxxxxxxx 			// login
    password: xxxxxxxx 			// and password to mqtt broker
    protocol: tls 				// or mqtt, default to tls
    address: xxxxxxxx 			// Broker IP or name, default to 127.0.0.1
    port: 8883 					// Port to connect to, could 1883 witout TLS, default to 8883
    certfile: /path/to/ca.crt 	// ca.crt file to enable TLS use
```

### Section Log

```
    format: ascii 	// could be also json
    output: stdout 	// could be stderr
    level: info 	// could be debug / / info / warning / error / fatal / panic
```

### Section Sensors

```
    -
        id: 2-3411604256	// Id du capteur.
        name: entree		// nom commun
    -
        id: 2-1103140112
        name: INCONNU1
    -
        ref: THGN132N-F		// Référence
        name: SdB_RdC 		// Nom commun
        id: 4-439195650		// Id
    ...
    ...
    ...

```

### Section Actuators

```
    -
        id: d2					// Code utilisé pour commander le périphérique
        name: pfsalon			// Nom commun
        protocol: rts			// Protocole
        topic: home/action/		// Topic MQTT sur lequel rfp2mqtt souscrit pour récupérer les commandes
        command: Up/My/Down		// Pour une utilisation future
    -
        id: a1
        name: prise
        protocol: dio
        topic: home/action/
        command: On/Off/Assoc

```

## Codification des Id

Les Id sont sous la forme pp-nnnnnnnn

Le nombre "pp" encode le protocole du périphérique ainsi :

```
	Protocole			Code
	
	VISONIC_433			1
	VISONIC_868			2
	CHACON_433			3
	DOMIA_433			4
	X10_433				5
	X2D_433				6
	X2D_868				7
	X2D_SHUTTER_868		8
	X2D_HA_ELEC_868		9
	X2D_HA_GAS_868		10
	SOMFY_RTS_433		11
	BLYSS_433			12
	PARROT_433_OR_868 	13
	FS20				14
	KD101_43			16
	FS20				22
	EDISIO				23
```

Le code nnnnnnnn est celui que le module RFPlayer renvoie dans ses trames.
