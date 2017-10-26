# Postal Services over Swarm

* Status of this document
* Core concepts
* Caveat
* API
  * Retrieve node information
  * Receive messages
  * Send messages using public key encryption
  * Send messages using symmetric encryption
  * Querying peer keys
  * Handshakes

`pss` enables message relay over swarm. This means nodes can send messages to each other without being directly connected with each other, while taking advantage of the efficient routing algorithms that swarm uses for transporting and storing data.

## STATUS OF THIS DOCUMENT

`pss` is under active development, and the first implementation is yet to be merged to the Ethereum main branch. Expect things to change.

Details on swarm routing and encryption schemes out of scope of this document.

Please refer to `ARCHITECTURE.md` for in-depth topics concerning `pss`.

## CORE CONCEPTS

Three things are required to send a `pss` message:

1. Encryption key
2. Topic
3. Message payload

*Encryption key* can be a public key or a 32 byte symmetric key. It must be coupled with a peer address in the node prior to sending.

*Topic* is the initial 4 bytes of a hash value.

*Message payload* is an arbitrary byte slice of data.

Upon sending the message it is encrypted and passed on from peer to peer. Any node along the route that can successfully decrypt the message is regarded as a recipient. Recipients continue to pass on the message to their peers, to make traffic analysis attacks more difficult.

The Address that is coupled with the encryption keys are used for routing the message. This does *not* need to be a full addresses; the network will route the message to the best of its ability with the information that is available. If *no* address is given (zero-length byte slice), routing is effectively deactivated, and the message is passed to all peers by all peers.

##

## API

Parameters and return values below have two alternate descriptions. The first is the value type expected when handling raw `json` values through the RPC. The second is the value type when the `rpc.Client` in `go-ethereum` is used.

### Retreive node information

`pss_getPublicKey`

Retrieves the public key of the node, in raw byte format

parameters:
none

returns:
1. publickey `base64(bytes)` `[]byte`

`pss_baseAddr`

Retrieves the swarm overlay address of the node, in raw byte format

parameters:
none

returns:
1. swarm overlay address `base64(bytes)` `[]byte`

`pss_stringToTopic`

Creates a deterministic 4 byte topic value from input

parameters:
1. topic string `string` `string`

returns:
1. pss topic `base64(bytes[4])` `pss.Topic`

### Receive messages

`pss_subscribe`

Creates a subscription. Received messages with matching topic will be passed to subscription client.

parameters:
1. "receive" (literal) `string` `string` 
2. topic `base64(bytes)` `pss.Topic`

returns:
1. subscription handle `base64(byte)` `rpc.ClientSubscription`

* In `golang` as special method is used:

`rpc.Client.Subscribe(context.Context, "pss", chan pss.APIMsg, "receive", pss.Topic)`

Incoming messages are encapsulated in an object (`pss.APIMsg` in `golang`) with the following members:

1. Msg (message payload) `base64(bytes)` `[]byte`
2. Asymmetric (true if encrypted with public key cryptography) `bool` `bool`
3. Key (raw encryption key in hex format) `string` `string`

### Send message using asymmetric encryption

`pss_setPeerPublicKey`

Register a peer's public key. This is done once for every topic that will be used with the peer. Address can be anything from 0 to 32 bytes inclusive of the peer's swarm overlay address.

parameters:
1. public key of peer `base64(bytes)` `[]byte`
2. topic `base64(bytes)` `pss.Topic`
3. address of peer `base64(bytes)` `pss.PssAddress`

returns:
none

`pss_sendAsym`

Encrypts the message using the provided public key, and signs it using the node's private key. It then wraps it in an envelope containing the topic, and sends it to the network. 

parameters:
1. public key of peer `base64(bytes)` `[]byte`
2. topic `base64(bytes)` `pss.Topic`
3. message `base64(bytes)` `[]byte`

returns:
none

### Send message using symmetric encryption

`pss_setSymmetricKey`

Register a symmetric key shared with a peer. This is done once for every topic that will be used with the peer. Address can be anything from 0 to 32 bytes inclusive of the peer's swarm overlay address.

If the fourth parameter is false, the key will *not* be added to the list of symmetric keys used for decryption attempts.

parameters:
1. symmetric key `base64(bytes)` `[]byte`
2. topic `base64(bytes)` `pss.Topic`
3. address of peer `base64(bytes)` `pss.PssAddress`
4. use for decryption `bool` `bool`

returns:
1. symmetric key id `string` `string`

`pss_sendSym`

Encrypts the message using the provided symmetric key, wraps it in an envelope containing the topic, and sends it to the network.

parameters:
1. symmetric key id `string` `string`
2. topic `base64(bytes)` `pss.Topic`
3. message `base64(bytes)` `[]byte`

returns:
none

### Querying peer keys

`pss_GetSymmetricAddressHint`

Return the swarm overlay address associated with the peer registered with the given symmetric key and topic combination.

parameters:
1. topic `base64(bytes)` `pss.Topic`
2. symmetric key id `string` `string`

returns:
1. peer address `base64(bytes)` `pss.PssAddress`

`pss_GetAsymmetricAddressHint`

Return the swarm overlay address associated with the peer registered with the given symmetric key and topic combination.

parameters:
1. topic `base64(bytes)` `pss.Topic`
2. public key in hex form `string` `string`

returns:
1. peer address `base64(bytes)` `pss.PssAddress`

### Handshakes

Convenience implementation of Diffie-Hellman handshakes using ephemeral symmetric keys. Peers keep separate sets of keys for incoming and outgoing communications.

*This functionality is an optional feature in `pss`. It is compiled in by default, but can be omitted by providing the `nopsshandshake` build tag.*

`pss_addHandshake`

Activate handshake functionality on the specified topic.

parameters:
1. topic to activate handshake on `base64(bytes)` `pss.Topic`

returns:
none

`pss_removeHandshake`

Remove handshake functionality on the specified topic.

parameters:
1. topic to remove handshake from `base64(bytes)` `pss.Topic`

returns:
none

`pss_handshake`

Instantiate handshake with peer, refreshing symmetric encryption keys.

parameters:
1. public key of peer in hex format `string` `string`
2. topic `base64(bytes)` `pss.Topic`
3. block calls until keys are received `bool` `bool`
4. flush existing incoming keys `bool` `bool`

returns:
1. list of symmetric keys `string[]` `[]string`\*

* If parameter 3 is false, the returned array will be empty.

`pss_getHandshakeKeys`

Get valid symmetric encryption keys for a specified peer and topic.

parameters:
1. public key of peer in hex format `string` `string`
2. topic `base64(bytes)` `pss.Topic`
3. include keys for incoming messages `bool` `bool`
4. include keys for outgoing messages `bool` `bool`

returns:
1. list of symmetric keys `string[]` `[]string`

`pss_getHandshakeKeyCapacity`

Get amount of remaining messages the specified key is valid for.

parameters:
1. symmetric key id `string` `string`

return:
1. number of messages `uint` `uint16`

`pss_getHandshakePublicKey`

Get the peer's public key associated with the specified symmetric key.

parameters:
1. symmetric key id `string` `string`

returns:
1. Associated public key in hex format `string` `string`

`pss_releaseHandshakeKey`

Invalidate the specified key.

Normally, the key will be kept for a grace period to allow for decryption of delayed messages. If instant removal is set, this grace period is omitted, and the key removed instantaneously.

parameters:
1. public key of peer in hex format `string` `string`
2. topic `base64(bytes)` `pss.Topic`
3. symmetric key id to release `string` `string`
4. remove keys instantly `bool` `bool`

returns:
1. whether key was successfully removed `bool` `bool`