# LumiTime - Embedded Gin server example

This project demonstrates a minimal Go server using the `gin` framework that serves an embedded HTML front-end (no front-end separation).

Files added:
- `main.go` - Gin server, embedded templates and static files, `/` and `/api/time` endpoints.
- `templates/index.html` - HTML page rendered by server.
- `static/style.css` - Styles for the page.

Run locally (macOS, zsh):

```zsh
go mod tidy
go run main.go
```

Then open `http://localhost:8080` in your browser.

To build a binary:

```zsh
go build -o lumitime
./lumitime
```
# About LumiTime
    [en]
    A time tracking website, welcome to join this project.    
    ....
    if you find this idea interesting, welcome to file a issue ticket and share your ideas~

* Plan & work to do
	[ ] Record the Lumi time history
	[ ] Easily transfrom raw schedule data to Lumi time JSON
	
    

