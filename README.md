LogAIS is designed to:
    • Receive AIS data via UDP
    • Write AIS sentences to file with timestamps

The file format is intended to be compatible with the OpenCPN VDR plugin for playback.
All timestamps are in UTC.  Data from each UDP port is written to a separate file, a new file is started for each port when UTC time rolls over to the next day.
Output files are organised in folders with year\month\day to facilitate finding specific events and ease of managing disk usage.

The program will also log its activity.
Some file permission errors give a "Please re-run installer" message, which will be more meaningfull when the installer is written.
