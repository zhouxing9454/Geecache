
del server.exe


go build -o server.exe


start server.exe -port=8001
start server.exe -port=8002
start server.exe -port=8003 -api=1


ping -n 2 127.0.0.1 > nul


echo ">>> start test"


start /B curl "http://localhost:9999/api?key=Tom"
start /B curl "http://localhost:9999/api?key=Tom"
start /B curl "http://localhost:9999/api?key=Tom"
start /B curl "http://localhost:9999/api?key=Tom"
start /B curl "http://localhost:9999/api?key=Tom"
start /B curl "http://localhost:9999/api?key=Tom"


ping -n 10 127.0.0.1 > nul

timeout /t 15

start /B curl "http://localhost:9999/api?key=Tom"
start /B curl "http://localhost:9999/api?key=Tom"
start /B curl "http://localhost:9999/api?key=Tom"
start /B curl "http://localhost:9999/api?key=Tom"
start /B curl "http://localhost:9999/api?key=Tom"
start /B curl "http://localhost:9999/api?key=Tom"


ping -n 10 127.0.0.1 > nul

echo ">>> test finished"