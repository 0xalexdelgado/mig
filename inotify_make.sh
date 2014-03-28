#!/bin/bash
# Julien Vehent - 2014
# watch the MIG source code directory and rebuild the components
# when a file is saved.

echo "starting inotify listener on ./src/mig"
# feed the inotify events into a while loop that creates
# the variables 'date' 'time' 'dir' 'file' and 'event'
inotifywait -mr --timefmt '%d/%m/%y %H:%M' --format '%T %w %f %e' \
-e modify ./src/mig \
| while read date time dir file event
do
    if [[ "$file" =~ \.go$ && "$dir" =~ src\/mig ]]; then
        dontskip=true
    else
        #echo skipping $date $time $event $dir $file
        continue
    fi

    #echo NEW EVENT: $event $dir $file

    if [[ "$dir" =~ src\/mig\/$ ]]; then
        make mig-agent && \
        make mig-action-generator && \
        make mig-action-verifier && \
        make mig-api && \
        make mig-scheduler && \
        echo success $(date +%H:%M:%S)

    elif [[ "$dir" =~ agent && "$file" != "configuration.go" ]] ; then
        make mig-agent && echo success $(date +%H:%M:%S)

    elif [[ "$dir" =~ api ]] ; then
        make mig-api && echo success $(date +%H:%M:%S)

    elif [[ "$dir" =~ generator ]] ; then
        make mig-action-generator && echo success $(date +%H:%M:%S)

    elif [[ "$dir" =~ verifier ]] ; then
        make mig-action-verifier && echo success $(date +%H:%M:%S)

    elif [[ "$dir" =~ pgp ]] ; then
        make mig-agent && \
        make mig-action-generator && \
        make mig-action-verifier && \
        make mig-api && \
        echo success $(date +%H:%M:%S)

    elif [[ "$dir" =~ scheduler ]] ; then
        make mig-scheduler && echo success $(date +%H:%M:%S)

    fi
done
