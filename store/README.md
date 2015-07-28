# store

Package store is a data store for everything that bmclient needs. Refer to godoc
for more docs.

Database structure:
```
- powQueue (bucket) (FIFO data structure)
-- 0x0000000000000001 (Queue entry #1)
--- Target (8 bytes) || Object Hash (64 bytes)

- pubkeyRequests (bucket)
-- BM-blahblahblah
--- number of requests (uint32) || last request time (binary time serialized 
      using time.MarshalBinary)

- misc (bucket)
-- dbMasterKey (encrypted)
-- salt
-- mailboxLatestID
-- powQueueLatestID
-- counters (bucket)
--- 0x00000000 (wire.ObjectTypeGetPubKey)
--- 0x00000002 (wire.ObjectTypeMsg)
--- 0x00000003 (wire.ObjectTypeBroadcast)

- mailboxes (bucket)
-- name (bucket)
--- data (bucket)
---- createdOn
--- 0x00000000000000010000000000000001 (Message of ID 1 and suffix 1)
---- Nonce (24 bytes) || Encrypted Contents

- broadcastAddresses (bucket)
-- BM-blahblahblah (no value)
```