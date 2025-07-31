package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/go-audio/wav"
	"github.com/tcolgate/mp3"
)

const (
	MaxAudioFileSize = 150 * 1024 * 1024 // 150MB limit
	ServerTimeout    = 30 * time.Second
)

// Metadata represents the metadata extracted from the audio file.
type Metadata struct {
	Duration   float64 `json:"duration"`
	SampleRate int     `json:"sample_rate"`
	Channels   int     `json:"channels"`
	Bitrate    int     `json:"bitrate"`
	FileSize   int64   `json:"file_size"`
}

// WaveformResponse represents the successful API response.
type WaveformResponse struct {
	Metadata      any `json:"metadata"`
	Audiowaveform any `json:"audiowaveform"`
}

// APIRequest represents the JSON request body.
type APIRequest struct {
	AudioURL        string  `json:"audio_url"`
	TotalPoints     int     `json:"total_points"`
	PointsPerSecond int     `json:"points_per_second"`
	Zoom            int     `json:"zoom"`
	Bits            int     `json:"bits"`
	SplitChannels   bool    `json:"split_channels"`
	AmplitudeScale  float64 `json:"amplitude_scale"`
}

func main() {
	http.HandleFunc("/waveform-metadata", waveformHandler)
	http.HandleFunc("/health", healthHandler)

	fmt.Println("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "waveform-metadata-api",
	})
}

func waveformHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST requests are allowed", http.StatusMethodNotAllowed)
		return
	}

	var params APIRequest
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, "Failed to decode JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if params.AudioURL == "" {
		http.Error(w, "Missing required 'audio_url' field", http.StatusBadRequest)
		return
	}

	if params.TotalPoints < 0 || params.PointsPerSecond < 0 || params.Zoom < 0 || params.AmplitudeScale < 0 {
		http.Error(w, "Negative values not allowed", http.StatusBadRequest)
		return
	}

	var audioData io.Reader
	var audioType string

	if strings.HasPrefix(params.AudioURL, "data:") {
		// Handle Base64 data URI
		parts := strings.SplitN(params.AudioURL, ",", 2)
		if len(parts) != 2 {
			http.Error(w, "Invalid data URI format", http.StatusBadRequest)
			return
		}

		header := parts[0]
		if strings.Contains(header, "audio/wav") {
			audioType = "wav"
		} else if strings.Contains(header, "audio/mpeg") {
			audioType = "mp3"
		} else {
			http.Error(w, "Unsupported media type in data URI. Please use 'audio/wav' or 'audio/mpeg'.", http.StatusUnsupportedMediaType)
			return
		}

		decoded, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			http.Error(w, "Failed to decode base64 audio data: "+err.Error(), http.StatusBadRequest)
			return
		}
		audioData = io.LimitReader(bytes.NewReader(decoded), MaxAudioFileSize)
	} else {
		// Handle URL
		lowerURL := strings.ToLower(params.AudioURL)

		// Disallow local/private URLs
		if strings.Contains(lowerURL, "localhost") || strings.Contains(lowerURL, "127.0.0.1") || strings.Contains(lowerURL, "10.") || strings.Contains(lowerURL, "192.168.") {
			http.Error(w, "Local/private URLs not allowed", http.StatusBadRequest)
			return
		}

		if strings.HasSuffix(lowerURL, ".wav") {
			audioType = "wav"
		} else if strings.HasSuffix(lowerURL, ".mp3") {
			audioType = "mp3"
		} else {
			http.Error(w, "Unsupported audio format from URL. Please use a URL ending in '.wav' or '.mp3'.", http.StatusUnsupportedMediaType)
			return
		}

		client := &http.Client{Timeout: ServerTimeout}
		resp, err := client.Get(params.AudioURL)
		if err != nil {
			http.Error(w, "Failed to fetch audio from URL: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer resp.Body.Close()
		audioData = io.LimitReader(resp.Body, MaxAudioFileSize)
	}

	tempFile, err := os.CreateTemp("", "audio-*.tmp")
	if err != nil {
		http.Error(w, "Failed to create temporary file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.Remove(tempFile.Name())

	if _, err := io.Copy(tempFile, audioData); err != nil {
		http.Error(w, "Failed to save audio data: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tempFile.Close()

	metadata, err := extractMetadata(tempFile.Name(), audioType)
	if err != nil {
		http.Error(w, "Failed to extract metadata: "+err.Error(), http.StatusInternalServerError)
		return
	}

	args := []string{
		"-i", tempFile.Name(),
		"--input-format", audioType,
		"--output-format", "json",
	}

	if params.TotalPoints > 0 {
		pps := float64(params.TotalPoints) / metadata.Duration
		args = append(args, "--pixels-per-second", fmt.Sprintf("%.2f", pps))
	} else if params.PointsPerSecond > 0 {
		args = append(args, "--pixels-per-second", strconv.Itoa(params.PointsPerSecond))
	} else if params.Zoom > 0 {
		args = append(args, "--zoom", strconv.Itoa(params.Zoom))
	}

	if params.Bits != 0 {
		args = append(args, "--bits", strconv.Itoa(params.Bits))
	}
	if params.SplitChannels {
		args = append(args, "--split-channels")
	}
	if params.AmplitudeScale > 0 {
		args = append(args, "--amplitude-scale", fmt.Sprintf("%.2f", params.AmplitudeScale))
	}

	cmd := exec.Command("audiowaveform", args...)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to execute audiowaveform: %s", stderr.String()), http.StatusInternalServerError)
		return
	}

	var waveformData any
	if err := json.Unmarshal(out.Bytes(), &waveformData); err != nil {
		http.Error(w, "Failed to parse waveform data from tool: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(WaveformResponse{
		Metadata:      metadata,
		Audiowaveform: waveformData,
	})
}

func extractMetadata(filePath string, audioType string) (*Metadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}

	metadata := &Metadata{
		FileSize: fileInfo.Size(),
	}

	file.Seek(0, 0)

	switch audioType {
	case "wav":
		d := wav.NewDecoder(file)
		if d == nil {
			return nil, fmt.Errorf("invalid wav file")
		}
		d.ReadInfo()
		duration, err := d.Duration()
		if err != nil {
			return nil, err
		}
		metadata.Duration = duration.Seconds()
		metadata.SampleRate = int(d.SampleRate)
		metadata.Channels = int(d.NumChans)
		metadata.Bitrate = int(d.AvgBytesPerSec * 8)
	case "mp3":
		decoder := mp3.NewDecoder(file)
		var frame mp3.Frame
		var skipped int
		var totalDuration time.Duration
		var firstFrame = true

		// Iterate through all frames to calculate duration
		for {
			err := decoder.Decode(&frame, &skipped)
			if err != nil {
				if err == io.EOF {
					break
				}
				if !firstFrame {
					// MP3 file may have trailing metadata, padding or corrupted frames
					break
				}
				return nil, err
			}

			// Get metadata from the first frame
			if firstFrame {
				header := frame.Header()
				metadata.SampleRate = int(header.SampleRate())

				// Determine channel count from channel mode
				channelMode := header.ChannelMode()
				switch channelMode {
				case mp3.SingleChannel:
					metadata.Channels = 1
				case mp3.Stereo, mp3.JointStereo, mp3.DualChannel:
					metadata.Channels = 2
				default:
					metadata.Channels = 2
				}

				firstFrame = false
			}

			// Add frame duration to total
			totalDuration += frame.Duration()
		}

		metadata.Duration = totalDuration.Seconds()

		// Calculate average bitrate
		if metadata.Duration > 0 {
			metadata.Bitrate = int((metadata.FileSize * 8) / int64(metadata.Duration))
		}
	default:
		return nil, fmt.Errorf("unsupported audio_type: %s", audioType)
	}

	return metadata, nil
}
