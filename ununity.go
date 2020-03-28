package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"github.com/urfave/cli/v2"
	"io"
	"os"
	"path"
	"strings"
)

func main() {
	app := cli.NewApp()

	app.Name = "Ununity"
	app.Action = unpack
	app.Description = "Extracts .unitypackage files into their normalized directory structure"
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name: "output",
			Aliases: []string{"o"},
			Required: false,
			Usage: "Destination output folder. Defaults to the name of the input archive without suffix",
		},
		&cli.BoolFlag{
			Name: "nometa",
			Required: false,
			Usage: "Does not write metadata files (.meta) alongside the asset files",
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		println(err.Error())
	}
}

func unpack(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("no path or file provided")
	}

	load := c.Args().First()

	f, err := os.Open(load)
	if err != nil {
		return fmt.Errorf("error opening file: %s", err.Error())
	}
	defer f.Close()

	outputDir := path.Dir(".")

	// Set the output to the base of the file if possible
	fileNoExt := strings.TrimSuffix(load, path.Ext(load))
	if path.Base(fileNoExt) != path.Base(load) {
		outputDir = path.Base(fileNoExt)
	}

	nometa := c.Bool("nometa")

	// If we provide -o we change output
	if c.IsSet("output") {
		outputDir = c.String("output")
	}

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("invalid package: %s", err.Error())
	}
	defer gz.Close()

	err = os.MkdirAll(outputDir, 0777)
	if err != nil {
		return fmt.Errorf("cannot create output directory: %s", err.Error())
	}

	pendingRenames := make(map[string]string) // hash => full path
	pendingMetaRenames := make(map[string]string) // hash => full path
	nameLookup := make(map[string]string) // hash => name

	fmt.Println("Extracting...")

	tf := tar.NewReader(gz)
	for {
		header, err := tf.Next()
		if err != nil {
			if err == io.EOF {
				break
			}

			return fmt.Errorf("error reading contents: %s", err.Error())
		}

		fname := path.Base(header.Name)
		hash := path.Dir(header.Name)

		if fname == hash || header.Size < 1 {
			continue
		}

		if fname == "asset" || (!nometa && fname == "asset.meta") {
			filename := hash
			if saved, found := nameLookup[hash]; found {
				filename = saved
			} else {
				// Mark as pending rename
				if fname == "asset.meta" {
					pendingMetaRenames[hash] = path.Join(outputDir, filename + ".meta")
				} else {
					pendingRenames[hash] = path.Join(outputDir, filename)
				}
			}

			// Append meta back to filename
			if fname == "asset.meta" {
				filename = filename + ".meta"
			}

			dst, err := os.Create(path.Join(outputDir, filename))
			if err != nil {
				return fmt.Errorf("error creating '%s': %s", path.Join(outputDir, filename), err.Error())
			}

			written, err := io.CopyN(dst, tf, header.Size)
			if err != nil || written != header.Size {
				return fmt.Errorf("error extracting '%s': %s", path.Join(outputDir, filename), err.Error())
			}
		} else if fname == "pathname" {
			pathBytes := make([]byte, header.Size)
			if _, err := io.ReadFull(tf, pathBytes); err != nil {
				return fmt.Errorf("error reading path info: %s", err.Error())
			}

			strpath := strings.TrimSpace(string(pathBytes))
			nameLookup[hash] = strpath

			fmt.Print("\rWriting ", strpath)

			// Check if this file was already written and thus awaiting rename
			if current, found := pendingRenames[hash]; found {
				if err := move(current, path.Join(outputDir, strpath)); err != nil {
					return err
				}
			}

			// Rename the meta
			if current, found := pendingMetaRenames[hash]; found {
				if err := move(current, path.Join(outputDir, strpath + ".meta")); err != nil {
					return err
				}
			}
		}
	}

	fmt.Println("\r\nExtracted successfully.")

	return nil
}

func move(current, targ string) error {
	// Create target folder
	if err := os.MkdirAll(path.Dir(targ), 0777); err != nil && err != os.ErrExist {
		return fmt.Errorf("error creating output '%s': %s", path.Dir(targ), err.Error())
	}

	if err := os.Rename(current, targ); err != nil {
		return fmt.Errorf("error moving file from %s to %s: %s", current, targ, err.Error())
	}

	return nil
}
