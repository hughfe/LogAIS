package main

/*
Write AIS data received by UDP to file in OpenCPN VDR format:
 Almost but not quite as documented here:
 https://opencpn-manuals.github.io/main/vdr/log_format.html

Reads a tab separated file of UDP port numbers and stream names - filename is hardcoded and must be in
C:\LogAIS (Windows) or
/var/local/LogAIS (Linux)
 output to same folder.
Input ports should not be repeated.

Spawns a separate go routine for each stream.
logs to %APPDATA%\LogAIS, file name below
Logfile rotated when size exceeds limit below, size checked at ticker interval below
*/

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

)

const (
	ConfName  = "LogAIS"    // config file name
	Lfsize    = 102400      // max size of Logfile before rotating it, (100KB)
	Logcheck  = 10          // minutes between checking Logfile size
	LogfName  = "LogAIS"
	Maxlogs   = 4           // number of old logfiles to keep
	Version   = "1.02"
)

var (
	Logfile       *os.File
	Logit         *log.Logger
	Logpath       = ""
	Datapath      = "" // output data path
	Sep           = ""
)

func abort(text string) {
	// called if unable to cd to datadir, don't know what will happen to call to log.
	//  tries to log event, ignore errors
	lname := LogfName + ".log"
	os.Chdir(Logpath) // home dir on Linux, no action on Windows
	lhandle, _ := os.OpenFile(lname, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0664)
	defer lhandle.Close()
	logx := log.New(lhandle, "UTC ", log.LUTC|log.LstdFlags|log.Lmsgprefix)
	logx.Fatal(text)
	fmt.Printf("Exiting program, error: %s\n", text)
	os.Exit(1)
}

func main() {
	var wg sync.WaitGroup

	// os specific variables
	switch runtime.GOOS {
	case "windows":
		Sep = "\\" // actual single backslash
		Datapath = "C:\\" + ConfName + Sep
		Logpath, _ = os.LookupEnv("APPDATA")
		Logpath += Sep + LogfName + Sep
	case "linux":
		Sep = "/"
		Datapath = "/var/local/" + ConfName + Sep
		Logpath = "/var/log/" + LogfName + Sep
	default:
		abort("Unknown OS: " + runtime.GOOS)
	}

	// find the dirs for config & log files
	if err := os.MkdirAll(Logpath, 0775); err != nil {
		// won't return
		abort("Fatal: unable to open logfile folder for logging: " + Logpath + " please rerun installer")
	}
	if err := os.Chdir(Datapath); err != nil {
		abort("Fatal: unable to set data directory to " + Datapath + " please rerun installer")
		return
	}

	// rotate Logfile now to start new file for each program launch
	rotateLog()
	// Logfile handle will change when Logfile is rotated, so will repeat this on exit (probably not necessary)
	defer Logfile.Close()
	Logit = log.New(Logfile, "UTC ", log.LUTC|log.LstdFlags|log.Lmsgprefix)
	Logit.Printf("LogAIS v%s started. CompAIS NZ", Version)

	conffile := Datapath + ConfName

	go logCheck()  // periodic check on logfile size

	// read file into memory
	content, err := os.ReadFile(conffile + ".txt")
	if err != nil {
		// file error, bail out
		abort("Fatal error reading " + conffile + ".txt : " + err.Error())
		return
	}

	// Break up content into lines
	afoArray := bytes.Split(content, []byte("\n"))
	for _, buf := range afoArray {
		// byte slice for each line
		// trim leading & trailing spaces, double spaces
		line := strings.TrimSpace(string(buf))
		line = strings.ReplaceAll(line, "  ", " ")
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if fields[0][0] == '#' {
			// ignore # comments
			continue
		}
		if len(fields) < 2 {
			// must have a description
			continue
		}

		// strip spaces except for description
		// any fields beyond 2 ignored
		text := make([]string, 2)
		for i := 0; i < 2; i++ {
			if i < 1 {
				fields[i] = strings.ReplaceAll(fields[i], " ", "")
			}
			text[i] = string(fields[i])
		}

		wg.Go(func() {
			startAIS(text, &Logit)
		})

//		go startAIS(text, &Logit)

	}

	Logit.Printf("Info: all channels started")

	fmt.Printf("%s Z\n", time.Now().UTC().Format(time.DateTime))
	fmt.Printf("\t\tAll processes started\n")
	fmt.Printf("\t\t****** DO NOT CLOSE THIS WINDOW! ******\n")
	fmt.Printf("\t\tunless the command prompt has returned!\n\n")

	wg.Wait()

	Logit.Printf("Exiting application.  Thank you for flying Coconut Airways.")
	defer Logfile.Close()
	return
}


func logCheck() {
	// repeat every 10 minutes
	for {
		time.Sleep(Logcheck * time.Minute)
		fstat, _ := Logfile.Stat()
		if fstat.Size() > Lfsize {
			rotateLog()
			Logit = log.New(Logfile, "UTC ", log.LUTC|log.LstdFlags|log.Lmsgprefix)
		}
	}
}

func checkPort(port string) (int, error) {
	num, err := strconv.Atoi(port)
	if err != nil {
		return 0, err
	}
	if num < 1025 || num > 65535 {
		return num, errors.New("Error port value out of range: " + port)
	}
	return num, nil
}

func rotateLog() {
	// rotates logfile up to the number specified in global variable
	// called at program startup and when the logfile gets to a size set in the main program
	// only checks for file permission errors, opens new logfile
	wd, err := os.Getwd()
	if err != nil {
		abort("Fatal: Can't get current folder\n")
		os.Exit(1)
	}
	if err = os.Chdir(Logpath); err != nil {
		abort("Fatal: Can't change folder for logging " + Logpath)
		os.Exit(1)
	}
	if err = os.Remove(LogfName + strconv.Itoa(Maxlogs) + ".log"); err != nil {
		// either file does not exist, or no permission to delete
		if errors.Is(err, os.ErrPermission) {
			abort("Fatal: Unable to delete old logfile: " + err.Error())
			os.Exit(1)
		}
	}

	for i := Maxlogs; i > 1; i-- {
		ai := strconv.Itoa(i)
		aj := strconv.Itoa(i - 1)
		if err = os.Rename(LogfName + aj + ".log", LogfName + ai + ".log"); err != nil {
			// only going to worry about file permision errors
			if errors.Is(err, os.ErrPermission) {
				abort("Fatal: Unable to rename old logfile: " + err.Error())
				os.Exit(1)
			}
		}
	}

	// close current logfile to rename it, then open new one
	Logfile.Close() // if there's an error it's either already closed or doesn't exist
	if err = os.Rename(LogfName + ".log", LogfName + "1.log"); err != nil {
		// only going to worry about file permision errors
		if errors.Is(err, os.ErrPermission) {
			abort("Fatal: Unable to rename old logfile: " + err.Error())
			os.Exit(1)
		}
	}

	// init new logfile
	Logfile, err = os.OpenFile(LogfName + ".log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0664)
	if err != nil {
		abort("Fatal: Could not open log file!")
		os.Exit(1)
	}
	// trap panics etc
	os.Stderr = Logfile

	os.Chdir(wd)
	return
}

func startAIS(line []string, logit **log.Logger) {
/*
	record data from one input port to file
	assume packets are clean enough...
*/

	var (
		bufsize                = 6144              // size of receive buffer
		filename               = " "
		loopwait time.Duration = (1 * time.Second) // seconds to wait for data before looping
		sockin                 *net.UDPConn
		spath                  = " "
		outfile                *os.File
	)

	fmt.Printf("Starting channel %s\n", line)

	input, err := checkPort(line[0])
	if err != nil {
		(*logit).Printf("Error: not a valid input port, skipping entry: %s", line[:])
		fmt.Printf("%s is not a valid port, skipping channel %s\n", line[0], line[1])
		return
	}

	// Connect to UDP source
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: input})
	if err != nil {
		(*logit).Printf("Error: %d can't connect to UDP input, error: %v", input, err)
		fmt.Printf("Can't connect to port %s, probably already in use, skipping channel\n", line)
		// Remote chance input port is already in use
		(*logit).Printf("Error: %d probably already in use, check input file", input)
		return
	}

	// UDP source connected
	(*logit).Printf("Info: %d connected for input", input)
	sockin = conn
	defer sockin.Close()

	buff := make([]byte, bufsize)
	npath := ""
	// loop forever listening for packets
	for {
		// get year, month, day, compare with previous
		year, mnth, day, rfctime := gettime()
		npath = Datapath + year + Sep + mnth + Sep + day + Sep
		if npath != spath {
			// date has changed or program restarted, close old file, ignore error if it doesn't exist
			outfile.Close()
			// new folder - no error if folder already exists
			if err = os.MkdirAll(npath, 0775); err != nil {
				(*logit).Printf("Fatal: unable to make output directory: %s, please rerun installer: %v", npath, err)
				return
			}
			// change folder
			if err = os.Chdir(npath); err != nil {
				(*logit).Printf("Fatal: unable to change dir to %s: %v", spath, err)
				return
			}

			filename = year + mnth + day + "-" + line[0] + ".csv"
			header := "# Restarted: " + rfctime + "\r\n"
			// check if file exists, might be restarting a recording.
			outfile, err = os.OpenFile(filename, os.O_WRONLY|os.O_APPEND, 0664)
			if err != nil {
				(*logit).Printf("Info: Creating new file: %s", filename)
				// file does not exist, create new
				outfile, err = os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0664)
				if err != nil {
					(*logit).Printf("Fatal: Could not open output file: %s: %v", filename, err)
					return
				}
				header = "# VDR Log File refer:\r\n" +
					"# https://opencpn-manuals.github.io/main/vdr/log_format.html\r\n" +
					"# Created: " + rfctime + "\r\n" +
					"# LogAIS.exe " + "\u00A9" + " CompAIS NZ Ltd\r\n" +
					"# NMEA0183 on UDP port " + line[0] + " \"" + line[1] + "\"\r\n" +
					"# received_at,protocol,msg_type,source,raw_data\r\n" +
					"# actual format in use differs from documented format:\r\n" +
					"timestamp,type,id,message\r\n"
			} else {
				(*logit).Printf("Info: Appending to file: %s", filename)
			}
			defer outfile.Close()

			if _, err = outfile.WriteString(header); err != nil {
				(*logit).Printf("Fatal: error writing to output file %s: %v", filename, err)
				outfile.Close()
				return
			}
			spath = npath
		}

		sockin.SetDeadline(time.Now().Add(loopwait))
		leng, err := sockin.Read(buff)
		if err != nil {
			// error reading from port
			if errors.Is(err, os.ErrDeadlineExceeded) {
				// loop on timeout
				continue
			}
			(*logit).Printf("Info: %d UDP read error: %+v", input, err)
			(*logit).Printf("Info: %d will re-open port", input)
			sockin.Close()
			conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: input})
			if err != nil {
				(*logit).Printf("Error: %d can't connect to UDP input, error: %v", input, err)
				return
			}
			// UDP source re-connected
			sockin = conn
			defer sockin.Close()
			(*logit).Printf("Info: %d input reconnected", input)
			continue
		} else {
			// no error, log big packets (input UDP)
			if leng > 1460 {
				(*logit).Printf("Info: %d large packet received %d bytes", input, leng)
			}
		}

		for i := 0; i+3 < leng; i++ {
			// need more than 3 bytes for a sentence, that's just to prevent out of range indeces
			if string(buff[i:(i+2)]) == "!A" {
				// start of a sentence, maybe
				// starting ! is at [i]
				j := i+1
				for ; j < leng && buff[j] != '*' && buff[j] != '!'; j++ {
//					could calculate checksum here
				}
				if j+3 > leng {
					// no ending checksum
					break
				}
				// if checksum '*' is missing, could be start of a new sentence
				// very unlikely though
				if buff[j] == '!' {
					// start of a new sentence
					i = j-1
					continue
				}
				// must be checksum marker '*'

				_, _, _, rfctime = gettime()
//				"timestamp,type,id,message"
				content := rfctime + ",AIS,\"UDP port:" + line[0] + "\",\"" + string(buff[i:(j+3)]) + "\"\r\n"
				if _, err = outfile.WriteString(content); err != nil {
					(*logit).Printf("Fatal: error writing to output file: %s - %s: %v", filename, content, err)
					outfile.Close()
					return
				}
				i = j+2
				// i also gets incremented at the end of the loop
			} // end found AIS sentence
		} // end loop through buffer
	} // end loop forever

	(*logit).Printf("Info: %d ending process for input port", input)
	return
}

func gettime() (string, string, string, string) {
	thetime := time.Now().UTC()
//	rfctime := thetime.Format(time.RFC3339) - doesn't do mS
	rfctime := thetime.Format("2006-01-02T15:04:05.000Z")
	texttime := strings.Split(thetime.Format("2006 01 02"), " ")
	return texttime[0], texttime[1], texttime[2], rfctime
}
