# Imagemage

Because apparently, Google decided that their Gemini image generation API needed its own bloated CLI tool - presumably to hit some product manager's OKR about "CLI adoption" or "developer engagement metrics." Meanwhile, those of us who just want to generate or edit an image without installing half of npm are left wondering why a simple API call requires more dependencies than a JavaScript project.

So here's Imagemage: a focused CLI tool that does exactly one thing - talks to Google's Gemini image API - without the unnecessary cruft. It's written in Go, which means it's a single binary. No package managers, no dependency hell, no telemetry phoning home about your prompt for "cyberpunk cat wearing sunglasses."

## What This Actually Does

Imagemage lets you generate and edit images using Google's Gemini models without requiring their official CLI or any of its questionable architectural decisions. You get:

- **Gemini 3 Pro Image** (default) - The good stuff: high-quality 4K generation
- **Gemini 2.5 Flash Image** (`--frugal`) - For when you're watching your API budget and don't need every pixel perfect

## What You Can Do With This Thing

- **Text-to-Image Generation** - Describe what you want, get an image. Revolutionary, I know.
- **Image Editing** - "Make the sky more dramatic" actually works. Also supports multi-image composition.
- **Photo Restoration** - For when your family photos look like they've been through a war.
- **Icon Generation** - Multiple sizes at once because manually resizing things is what we did in the before times.
- **Pattern Creation** - Seamless patterns and textures without opening Photoshop.
- **Visual Storytelling** - Sequential images for when one picture isn't worth enough words.
- **Technical Diagrams** - Yes, you can generate flowcharts with AI now. No, I don't know if that's a good idea either.

## Prerequisites

- Go 1.22 or higher (because Go is civilized and doesn't make you manage Python versions)
- A Google Gemini API key (the actual cost of using this)

## Installation

### Homebrew (Recommended)

```bash
brew tap quinnypig/imagemage
brew install imagemage
```

That's it. One tap, one install. Like installing software should be.

### From Binary Releases

Download pre-built binaries for your platform from the [releases page](https://github.com/quinnypig/imagemage/releases).

Available for:
- macOS (Intel & Apple Silicon)
- Linux (amd64 & arm64)
- Windows (amd64)

For Linux/macOS:
```
VER=$(curl -s https://api.github.com/repos/quinnypig/imagemage/releases/latest | jq -r '.tag_name')
curl -sL https://github.com/quinnypig/imagemage/releases/download/${VER}/imagemage_${VER#v}_$(uname)_$(uname -m).tar.gz| tar -C /usr/local/bin -xzv imagemage
```

### From Source

```bash
# Clone this repository
git clone https://github.com/quinnypig/imagemage.git
cd imagemage

# Build it (produces a single binary, like Go intended)
go build -o imagemage

# Optionally, install to your $GOPATH/bin
go install
```

No `npm install`, no virtual environments, no "did you activate your venv?" Just a binary.

### Configuration

Set your Gemini API key as an environment variable. Because we're not barbarians who hardcode credentials, Imagemage checks for these in order:

```bash
export NANOBANANA_GEMINI_API_KEY="your-api-key-here"
# or
export NANOBANANA_GOOGLE_API_KEY="your-api-key-here"
# or
export GEMINI_API_KEY="your-api-key-here"
# or
export GOOGLE_API_KEY="your-api-key-here"
```

(Yes, the env vars still say NANOBANANA. They're the standard names used across Gemini image tools. Don't @ me.)

Get your API key from [Google AI Studio](https://makersuite.google.com/app/apikey). Yes, you'll need a Google account. No, there's no way around it.

## Usage

### Generate Command

The basic use case: turn words into pictures. Shockingly straightforward.

```bash
# Basic generation - describe what you want
imagemage generate "watercolor painting of a fox in snowy forest"

# Generate multiple variations (for when you're indecisive)
imagemage generate "mountain landscape" --count=3

# Specify output directory and style
imagemage generate "cyberpunk city" --output=./images --style="neon, futuristic"

# Control aspect ratio (because not everything is square)
imagemage generate "wide cinematic landscape" --aspect-ratio="21:9"
imagemage generate "phone wallpaper" --aspect-ratio="9:16"
imagemage generate "social media post" --aspect-ratio="1:1"

# Use frugal mode when you're watching your API costs
imagemage generate "concept art" --frugal --count=5
```

**Useful Flags:**
- `-c, --count` - Number of images to generate (default: 1)
- `-o, --output` - Output directory (default: current directory, because obviously)
- `-s, --style` - Additional style guidance for when your prompt needs more... guidance
- `-a, --aspect-ratio` - Aspect ratio (1:1, 16:9, 9:16, 4:3, 3:4, 3:2, 2:3, 21:9, 5:4, 4:5)
- `-f, --frugal` - Use the cheaper Flash model instead of Pro (your wallet will thank you)
- `--slide` - Optimized for presentation slides (4K, 16:9)
- `--store-prompt` - Save the prompt in the image metadata (for reproducibility)

**Supported Aspect Ratios:**
- **Square:** 1:1 (1024x1024) - The default, for some reason
- **Landscape:** 16:9 (1344x768), 4:3, 3:2, 21:9 - For when you want it wider
- **Portrait:** 9:16 (768x1344), 3:4, 2:3 - For when you want it taller
- **Other:** 5:4, 4:5 - For the rebels among us

**Pro tip:** Use the `--aspect-ratio` flag instead of mentioning dimensions in your prompt. The model is terrible at understanding "make it 1920x1080" but great at understanding `--aspect-ratio="16:9"`.

### Edit Command

Take an existing image and modify it with natural language. Also supports multi-image composition, because sometimes you need to Photoshop things together but don't want to learn Photoshop.

```bash
# Basic editing - actually works surprisingly well
imagemage edit photo.png "make it black and white"

# Add elements that weren't there
imagemage edit landscape.png "add a rainbow in the sky"

# Change the background entirely
imagemage edit portrait.png "change background to beach" --output=edited.png

# Compose multiple images (up to 14, but best results with 3 or fewer)
imagemage edit background.png "add this person on the left" -i person.png
imagemage edit scene.png "put these people here" -i person1.png -i person2.png
```

**Flags:**
- `-o, --output` - Output path for edited image (default: input_edited.png)
- `-i, --input` - Additional images to compose (can be used multiple times)
- `-f, --frugal` - Use the cheaper model (when perfection isn't the goal)
- `--force` - Overwrite existing files without asking (live dangerously)
- `--store-prompt` - Save your edit instruction in the metadata

### Restore Command

For when your precious family photos look like they've been stored in a damp basement for 40 years.

```bash
# Restore an old photo (results may vary)
imagemage restore old_photo.png

# Specify where to save it
imagemage restore damaged.jpg --output=restored.png
```

**Flags:**
- `-o, --output` - Output path for restored image

### Icon Command

Generate app icons in multiple sizes at once. Because manually resizing the same image 8 times is what we did in 2005.

```bash
# Generate an app icon
imagemage icon "coffee cup logo"

# Specify exactly which sizes you need
imagemage icon "rocket ship" --sizes="64,128,256,512" --type="app-icon"

# Generate UI elements
imagemage icon "hamburger menu" --type="ui-element"
```

**Flags:**
- `--sizes` - Comma-separated list of sizes in pixels (default: "64,128,256")
- `--type` - Icon type: app-icon, favicon, ui-element (default: "app-icon")
- `-o, --output` - Output directory

### Pattern Command

Create seamless patterns and textures without opening Adobe Creative Cloud and waiting for it to update.

```bash
# Generate a geometric pattern
imagemage pattern "geometric triangles"

# Add some style to it
imagemage pattern "floral" --style="watercolor"

# Minimal and modern, like your design aesthetic
imagemage pattern "hexagons" --style="minimal, modern"
```

**Flags:**
- `--type` - Pattern type: seamless, tiled, texture (default: "seamless")
- `-s, --style` - Pattern style
- `-o, --output` - Output directory

### Story Command

Generate sequential images for visual storytelling. Like a storyboard, but you don't need to hire a storyboard artist.

```bash
# Create a growth sequence
imagemage story "a seed growing into a tree" --frames=4

# Time progression
imagemage story "day to night transition in a city" --frames=6 --style="cinematic"

# Character transformation
imagemage story "caterpillar to butterfly" --frames=3
```

**Flags:**
- `-f, --frames` - Number of frames (default: 3, min: 2, max: 10)
- `-s, --style` - Visual style for consistency across frames
- `-o, --output` - Output directory

### Diagram Command

Generate technical diagrams and flowcharts. I know what you're thinking: "Can AI really generate useful technical diagrams?" Try it and find out.

```bash
# Create a flowchart
imagemage diagram "CI/CD pipeline with testing stages"

# Architecture diagram (for your microservices mess)
imagemage diagram "microservices architecture" --type="architecture"

# Sequence diagram
imagemage diagram "user authentication flow" --type="flowchart"
```

**Flags:**
- `--type` - Diagram type: flowchart, architecture, sequence, entity-relationship (default: "diagram")
- `-o, --output` - Output directory

## Project Structure

```
imagemage/
├── main.go                 # Application entry point
├── cmd/                    # Command implementations
│   ├── root.go            # Root command and CLI setup
│   ├── generate.go        # Text-to-image generation
│   ├── edit.go            # Image editing
│   ├── restore.go         # Photo restoration
│   ├── icon.go            # Icon generation
│   ├── pattern.go         # Pattern creation
│   ├── story.go           # Sequential image generation
│   └── diagram.go         # Diagram generation
├── pkg/
│   ├── gemini/            # Gemini API client
│   │   └── client.go
│   └── filehandler/       # File handling utilities
│       └── filehandler.go
├── go.mod                 # Go module definition
└── README.md             # This file
```

## How It Works

It's refreshingly simple, actually:

1. **API Client** (`pkg/gemini`): Handles authentication and talks to Google's Gemini API directly
2. **Request Formation**: Your prompt becomes a JSON request. No middleware, no abstraction layers, no "enterprise service mesh."
3. **Response Processing**: Images come back as base64-encoded data, get decoded, done.
4. **File Management** (`pkg/filehandler`): Generates sane filenames and saves files without overwriting things accidentally
5. **Commands**: Each command is a focused interface for a specific task. No feature creep, no "we added AI to your AI."

## When Things Go Wrong

The tool provides actually useful error messages for common issues:

- **Invalid API Key**: Your API key is wrong, missing, or you forgot to export it. Check your environment variables.
- **API Quota Exceeded**: You've hit Google's rate limits. Either wait, or upgrade your quota. Or use `--frugal` more often.
- **Safety Concerns**: The content filter rejected your prompt. Try rephrasing it, or don't try to generate that.
- **Network Errors**: Your internet is down, Google's API is down, or something in between is down. Check accordingly.

## Development

### Building

```bash
go build -o imagemage
```

That's it. It's Go. It just works.

### Testing

```bash
go test ./...
```

### Adding New Commands

1. Create a new file in `cmd/` (e.g., `cmd/yourcommand.go`)
2. Implement the command using Cobra's structure (look at existing commands for examples)
3. Register it in the `init()` function
4. Update this README
5. Submit a PR

## Why "Imagemage"?

Because you're conjuring images like a wizard (mage), and the name isn't trademarked by Google. Simple as that.

(The environment variables still reference NANOBANANA because that's the convention established by the community tools for Gemini's image API. Consistency matters more than ego.)

## Contributing

Found a bug? Have a feature request? Want to make the snark even snarkier?

PRs are welcome. Issues are welcome. Constructive criticism is welcome.

Just keep it civil and remember: this is a tool for generating images with an API, not a framework for revolutionizing enterprise image generation paradigms.

## License

MIT License - see LICENSE file for details. Use it, fork it, deploy it, sell it to your enterprise for six figures. I don't care. Just don't blame me when the AI generates something weird.

## Acknowledgments

- Google, for making an image generation API that's actually pretty good
- The Go team, for making a language where "just compile it" actually works
- [Cobra](https://github.com/spf13/cobra), for making CLI development not painful
- [gemimg](https://github.com/minimaxir/gemimg) by Max Woolf - inspired several features including storing prompts in metadata (genius!)
- Everyone who said "just use the official Gemini CLI" - you inspired this out of spite
