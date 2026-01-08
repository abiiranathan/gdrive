# gdrive

A comprehensive Go client library for Google Drive API v3 with support for file operations, folder management, streaming, partial downloads, and Google Workspace document exports.

## Features

- 📁 **File Operations**: List, upload, download, and stream files
- 🗂️ **Folder Management**: Create folders, list files in folders with full path resolution
- 🔄 **Streaming Support**: Efficient streaming for large files
- 📊 **Partial Downloads**: Resume downloads and stream file chunks
- 📄 **Workspace Documents**: Export Google Docs, Sheets, Slides to various formats
- 🔐 **Multiple Auth Methods**: OAuth2 and Service Account support
- 🗑️ **Trash Operations**: Move files to trash and restore them
- 🔒 **Thread-Safe**: Safe for concurrent use

## Installation

```bash
go get github.com/abiiranathan/gdrive
```

## Prerequisites

You need Google Drive API credentials. Choose one:

1. **OAuth2 Credentials** (for user authentication)
   - Go to [Google Cloud Console](https://console.cloud.google.com)
   - Create a project and enable Google Drive API
   - Create OAuth2 credentials
   - Download the credentials JSON file

2. **Service Account** (for server-to-server)
   - Create a service account in Google Cloud Console
   - Download the service account JSON key
   - Share Drive files/folders with the service account email

## Quick Start

### OAuth2 Authentication

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/abiiranathan/gdrive"
    "golang.org/x/oauth2"
)

func main() {
    ctx := context.Background()
    
    // Read OAuth2 credentials
    credentials, err := os.ReadFile("credentials.json")
    if err != nil {
        log.Fatal(err)
    }
    
    // Get OAuth2 config
    config, err := gdrive.GetConfigFromJSON(credentials)
    if err != nil {
        log.Fatal(err)
    }
    
    // Get token (implement OAuth2 flow)
    token := &oauth2.Token{
        AccessToken: "your-access-token",
        // ... other token fields
    }
    
    // Create client
    client, err := gdrive.NewDriveClientWithToken(ctx, config, token)
    if err != nil {
        log.Fatal(err)
    }
    
    // List files
    files, err := client.ListFiles(ctx)
    if err != nil {
        log.Fatal(err)
    }
    
    for _, file := range files {
        fmt.Printf("%s (%s) - %s\n", file.Name, file.ID, file.FolderPath)
    }
}
```

### Service Account Authentication

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/abiiranathan/gdrive"
)

func main() {
    ctx := context.Background()
    
    // Read service account credentials
    credentials, err := os.ReadFile("service-account.json")
    if err != nil {
        log.Fatal(err)
    }
    
    // Create client
    client, err := gdrive.NewDriveClientForServiceAccount(ctx, credentials)
    if err != nil {
        log.Fatal(err)
    }
    
    // Use client...
}
```

## Usage Examples

### Listing Files

```go
// List all files
files, err := client.ListFiles(ctx)
if err != nil {
    log.Fatal(err)
}

for _, file := range files {
    fmt.Printf("Name: %s\n", file.Name)
    fmt.Printf("ID: %s\n", file.ID)
    fmt.Printf("Size: %d bytes\n", file.Size)
    fmt.Printf("MIME Type: %s\n", file.MimeType)
    fmt.Printf("Path: %s\n", file.FolderPath)
    fmt.Printf("Web Link: %s\n\n", file.WebViewLink)
}

// List files in specific folder
files, err := client.ListFilesInFolder(ctx, "folder-id")
```

### Uploading Files

```go
// Upload from file path
fileID, err := client.UploadFile(ctx, "/path/to/file.pdf", "file.pdf", "")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Uploaded file ID: %s\n", fileID)

// Upload to specific folder
fileID, err := client.UploadFile(ctx, "/path/to/file.pdf", "file.pdf", "folder-id")

// Upload from io.Reader (useful for web uploads)
file, _ := os.Open("document.pdf")
defer file.Close()

fileID, err := client.UploadFileFromReader(ctx, file, "document.pdf", "application/pdf", "")
```

### Downloading Files

```go
// Download to file
bytesWritten, err := client.DownloadFile(ctx, "file-id", "/path/to/output.pdf")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Downloaded %d bytes\n", bytesWritten)

// Stream to io.Writer (e.g., HTTP response)
var buf bytes.Buffer
bytesWritten, err := client.StreamFile(ctx, "file-id", &buf)
```

### Partial Downloads

```go
// Download specific byte range (useful for resumable downloads)
var buf bytes.Buffer
bytesWritten, err := client.PartialStreamFile(ctx, "file-id", &buf, 0, 1023)
if err != nil {
    log.Fatal(err)
}
// Downloads first 1024 bytes

// Or use PartialDownloadOptions
opts := gdrive.PartialDownloadOptions{
    StartByte: 1024,
    EndByte:   2047,
}
bytesWritten, err := client.PartialDownloadFile(ctx, "file-id", &buf, opts)
```

### Exporting Google Workspace Documents

```go
// Check if file is a Workspace document
isWorkspace, err := client.IsWorkspaceDocument(ctx, "file-id")
if err != nil {
    log.Fatal(err)
}

if isWorkspace {
    // Export to PDF
    var buf bytes.Buffer
    bytesWritten, err := client.ExportWorkspaceDocument(ctx, "file-id", &buf, gdrive.ExportFormatPDF)
    if err != nil {
        log.Fatal(err)
    }
    
    // Export to file
    bytesWritten, err := client.ExportWorkspaceDocumentToFile(
        ctx, 
        "file-id", 
        "/path/to/output.pdf", 
        gdrive.ExportFormatPDF,
    )
    
    // Get available export formats
    exportLinks, err := client.GetExportLinks(ctx, "file-id")
    for mimeType, link := range exportLinks {
        fmt.Printf("%s: %s\n", mimeType, link)
    }
}
```

### Available Export Formats

```go
// PDF
gdrive.ExportFormatPDF

// Microsoft Office
gdrive.ExportFormatDOCX  // Word
gdrive.ExportFormatXLSX  // Excel
gdrive.ExportFormatPPTX  // PowerPoint

// OpenDocument
gdrive.ExportFormatODT  // Text
gdrive.ExportFormatODS  // Spreadsheet
gdrive.ExportFormatODP  // Presentation

// Other formats
gdrive.ExportFormatRTF
gdrive.ExportFormatTXT
gdrive.ExportFormatHTML
gdrive.ExportFormatCSV
gdrive.ExportFormatEPUB
gdrive.ExportFormatJPEG
gdrive.ExportFormatPNG
gdrive.ExportFormatSVG
gdrive.ExportFormatZIP
```

### Folder Operations

```go
// Create folder
folderID, err := client.CreateFolder(ctx, "My Folder", "")
if err != nil {
    log.Fatal(err)
}

// Create subfolder
subfolderID, err := client.CreateFolder(ctx, "Subfolder", folderID)
```

### Trash Operations

```go
// Move file to trash (recoverable)
err := client.TrashFile(ctx, "file-id")
if err != nil {
    log.Fatal(err)
}

// Restore from trash
err := client.RestoreFile(ctx, "file-id")
if err != nil {
    log.Fatal(err)
}

// Permanently delete (irreversible!)
err := client.DeleteFile(ctx, "file-id")
if err != nil {
    log.Fatal(err)
}
```

### Working with File Revisions

```go
// Download specific revision (must be marked "Keep Forever")
var buf bytes.Buffer
bytesWritten, err := client.DownloadRevision(ctx, "file-id", "revision-id", &buf)

// Partial download of revision
opts := gdrive.PartialDownloadOptions{
    StartByte: 0,
    EndByte:   1023,
}
bytesWritten, err := client.PartialDownloadRevision(ctx, "file-id", "revision-id", &buf, opts)
```

## API Reference

### Types

#### `DriveClient`
Main client for interacting with Google Drive API. Thread-safe for concurrent use.

#### `FileInfo`
```go
type FileInfo struct {
    ID          string   // Unique file identifier
    Name        string   // File name
    MimeType    string   // MIME type
    Size        int64    // Size in bytes
    WebViewLink string   // Browser view URL
    Parents     []string // Parent folder IDs
    FolderPath  string   // Full path (e.g., "My Drive/Projects/2024")
}
```

#### `PartialDownloadOptions`
```go
type PartialDownloadOptions struct {
    StartByte int64 // Starting byte position (inclusive)
    EndByte   int64 // Ending byte position (inclusive)
}
```

### Client Creation

- `NewDriveClientForServiceAccount(ctx, credentials)` - Create client with service account
- `NewDriveClientWithToken(ctx, config, token)` - Create client with OAuth2 token
- `GetConfigFromJSON(credentials)` - Parse OAuth2 config from JSON

### File Operations

- `ListFiles(ctx)` - List all files with folder paths
- `ListFilesInFolder(ctx, folderID)` - List files in specific folder
- `UploadFile(ctx, filePath, fileName, parentFolderID)` - Upload file
- `UploadFileFromReader(ctx, reader, fileName, mimeType, parentFolderID)` - Upload from reader
- `DownloadFile(ctx, fileID, outputPath)` - Download to file
- `StreamFile(ctx, fileID, writer)` - Stream to io.Writer
- `PartialStreamFile(ctx, fileID, writer, startByte, endByte)` - Partial download
- `PartialDownloadFile(ctx, fileID, writer, opts)` - Partial download with options

### Folder Operations

- `CreateFolder(ctx, folderName, parentFolderID)` - Create folder

### Trash Operations

- `TrashFile(ctx, fileID)` - Move to trash
- `RestoreFile(ctx, fileID)` - Restore from trash
- `DeleteFile(ctx, fileID)` - Permanently delete

### Google Workspace Operations

- `IsWorkspaceDocument(ctx, fileID)` - Check if file is Workspace document
- `ExportWorkspaceDocument(ctx, fileID, writer, format)` - Export to format
- `ExportWorkspaceDocumentToFile(ctx, fileID, outputPath, format)` - Export to file
- `GetExportLinks(ctx, fileID)` - Get available export formats

### Revision Operations

- `DownloadRevision(ctx, fileID, revisionID, writer)` - Download revision
- `PartialDownloadRevision(ctx, fileID, revisionID, writer, opts)` - Partial revision download

## Error Handling

All methods return errors that should be checked. Errors are wrapped with context using `fmt.Errorf` with `%w` for error unwrapping.

```go
files, err := client.ListFiles(ctx)
if err != nil {
    // Handle error
    log.Printf("Failed to list files: %v", err)
    return
}
```

## Limitations

- Maximum page size: 100 files per request
- Partial downloads not supported for Google Workspace documents
- Exported Workspace documents limited to 10 MB
- Revision downloads require revision to be marked "Keep Forever"
- Folder path resolution limited to 10 levels (prevents infinite loops)

## Dependencies

```
golang.org/x/oauth2
google.golang.org/api/drive/v3
google.golang.org/api/option
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MIT License - see LICENSE file for details

## Author

Abiira Nathan (@abiiranathan)

## Links

- [Google Drive API Documentation](https://developers.google.com/drive/api/v3/reference)
- [Google Cloud Console](https://console.cloud.google.com)
- [OAuth2 Flow Guide](https://developers.google.com/identity/protocols/oauth2)