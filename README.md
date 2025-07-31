# Waveform-Metadata-API

A simple REST API for generating audio waveform data and extracting metadata from MP3 and WAV files using the [bbc/audiowaveform](https://github.com/bbc/audiowaveform) tool.

## Build and Run

### Using Docker

Build the Docker image:
```bash
docker build -t waveform-metadata-api .
```

Run the container:
```bash
docker run -p 8080:8080 waveform-metadata-api
```

The API will be available at `http://localhost:8080`.

### Local Development
Prerequisites: You need to have Go 1.24 or later installed, as well as the BBC audiowaveform tool.

Install dependencies:
```bash
go mod download
```

Run:
```bash
go run main.go
```

## API Specification

### **POST** `/waveform-metadata`

Generates waveform data and extracts metadata from an audio file.

**Request Body:**
```json
{
  "audio_url": "string",
  "total_points": 0,
  "points_per_second": 0,
  "zoom": 0,
  "bits": 0,
  "split_channels": false,
  "amplitude_scale": 0.0
}
```

#### Request Parameters

- **`audio_url`** (_string_, **required**): Audio file URL or base64 data URI. Supported formats: MP3, WAV.
- **`total_points`** (_integer_, *optional*): Total number of waveform data points to generate. When specified, calculates points_per_second automatically based on audio duration. Mutually exclusive with points_per_second and zoom.
- **`points_per_second`** (_integer_, *optional*, default: `100`): Number of output waveform data points per second of audio. Used only if total_points is not specified.
- **`zoom`** (_integer_, *optional*): Number of input samples per output waveform data point. Used only if neither total_points nor points_per_second is specified.
- **`bits`** (_integer_, *optional*, default: `16`): Number of data bits for output waveform data points. Valid values: 8 or 16.
- **`split_channels`** (_boolean_, *optional*, default: `false`): Output multi-channel data instead of combining into single waveform.
- **`amplitude_scale`** (_number_, *optional*, default: `1.0`): Amplitude scaling factor to apply to the waveform.

#### Audio Input Formats

**URL Format:**
```json
{
  "audio_url": "https://example.com/audio.mp3"
}
```

**Base64 Data URI Format:**
```json
{
  "audio_url": "data:audio/wav;base64,UklGRigAAABXQVZFZm10..."
}
```

Supported MIME types for data URIs:
- `audio/wav` for WAV files
- `audio/mpeg` for MP3 files

#### Response

**Success (200):**
```json
{
  "metadata": {
    "duration": 120.5,
    "sample_rate": 44100,
    "channels": 2,
    "bitrate": 320000,
    "file_size": 4836234
  },
  "audiowaveform": {
    "version": 2,
    "channels": 2,
    "sample_rate": 44100,
    "samples_per_pixel": 256,
    "bits": 16,
    "length": 4717,
    "data": [0, 1, 2, ...]
  }
}
```

**Metadata Fields:**
- `duration`: Audio duration in seconds
- `sample_rate`: Audio sample rate in Hz
- `channels`: Number of audio channels
- `bitrate`: Audio bitrate in bits per second
- `file_size`: File size in bytes

**Error Responses:**

| Status Code | Description |
|-------------|-------------|
| 400 | Bad Request - Invalid parameters, missing audio_url, or malformed data |
| 405 | Method Not Allowed - Only POST requests accepted |
| 415 | Unsupported Media Type - Unsupported audio format |
| 500 | Internal Server Error - Processing error or audiowaveform execution failure |

