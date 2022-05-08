# go-websocket-pusher-example-api

Run the Server:
```
cd cmd/chat-api
go run .
```

Open the browser:
- http://localhost
- Enter UserID and Room Name, then Register to get a SessionID.
- Users can be assigned to multiple rooms upon registration (but the HTML page only sends one currently).
- Initiate a Chat Message (over HTTP), or a System Message (for convenience of example).
- User a second browser tab to simulate a different user.
- After connecting, USers get a sessionID.
- All communication with the Server requires that sessionID, since we don't want the same client to be able to carry on multiple concurrent connections.

Details:
- What happens when a User joins a set of Rooms (Topics)?
    + Each topic receives a new System Message saying that the USer has joined.
- What happens when a User Re-registers? 
    + A User can only have 1 connection. New connection requests yield a new SessionID. Old connections are closed. Each HTTP request must be accompanied by a sessionID.