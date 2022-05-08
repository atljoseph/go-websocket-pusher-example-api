# go-websocket-pusher-example-api

Run the Server:
```
cd cmd/chat-api
go run .
```

Open the browser:
- http://localhost
- Enter UserID and Room Name, then Register to get a UserID.
- Initiate a System or Chat Message.
- User a second browser tab to simulate a different user.
- After connecting, USers get a sessionID.
- All communication with the Server requires that sessionID, since we don't want the same client to be able to carry on multiple concurrent connections.