# Goodreads Crawler

### How to run

    go run main.go -url <goodreads_book_url> -depth <depth_to_search> -workers <workers_to_get_books> -target <json_file_to_write>

### Output location

    output.json

### How to create binary

    go build

### How to run binary

    ./goodreads -url <goodreads_book_url> -depth <depth_to_search> -workers <workers_to_get_books> -target <json_file_to_write>

### Installing Go
##### For macOS

    brew install go

##### For Ubuntu

    sudo apt update
    sudo apt install golang-go

##### For Arch

    sudo pacman -S go

##### For Windows

    1. Download the Go installer from the official Go website.
    2. Run Go installer and follow the on-screen instructions.


### To format output.json in vscode

    Ctrl + Shift + I