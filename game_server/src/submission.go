package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"game/db"
	"game/log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/uptrace/bun"
)

type SubResp struct {
	Msg    string `json:"msg"`
	Flag   string `json:"flag"`
	Status string `json:"status"`
}

// Crate a map of lock for each team
var lockMap map[string]*sync.RWMutex = make(map[string]*sync.RWMutex)
var lockMappingMutex sync.Mutex
var lastSubmissionTime map[string]time.Time = make(map[string]time.Time)
var scoreMutex sync.Mutex

var scale float64 = 15 * math.Sqrt(5.0)
var norm float64 = math.Log(math.Log(5.0)) / 12.0

func elaborateFlag(team *TeamInfo, flag string, resp *SubResp, round uint) {
	var ctx context.Context = context.Background()
	info := new(db.Flag)
	err := conn.NewSelect().Model(info).Where("id = ?", strings.Trim(flag, " \n\t\r")).Scan(ctx)
	if err != nil {
		resp.Msg = fmt.Sprintf("[%s] Denied: invalid flag", flag)
		resp.Status = "DENIED"
		log.Debugf("Flag %s from %s: invalid", flag, team)
		return
	}
	if team == nil {
		resp.Msg = fmt.Sprintf("[%s] Denied: invalid team", flag)
		resp.Status = "DENIED"
		log.Debugf("Flag %s from %s: invalid team", flag, team)
		return
	}
	if team.Nop {
		resp.Msg = fmt.Sprintf("[%s] Denied: flag from nop team", flag)
		resp.Status = "DENIED"
		log.Debugf("Flag %s from %s: from nop team", flag, team)
		return
	}
	teamIP := teamIDToIP(team.ID)
	if info.Team == teamIP {
		resp.Msg = fmt.Sprintf("[%s] Denied: flag is your own", flag)
		resp.Status = "DENIED"
		log.Debugf("Flag %s from %s: is your own", flag, team)
		return
	}
	if int64(round)-int64(info.Round) >= int64(conf.FlagExpireTicks) {
		resp.Msg = fmt.Sprintf("[%s] Denied: flag too old", flag)
		resp.Status = "DENIED"
		log.Debugf("Flag %s from %s: too old", flag, team)
		return
	}
	flagSubmission := new(db.FlagSubmission)
	if err = conn.NewSelect().Model(flagSubmission).Where("team = ? and flag_id = ?", teamIP, info.ID).Scan(ctx); err != nil {
		if err != sql.ErrNoRows {
			log.Panicf("Error fetching flag submission: %v", err)
			resp.Msg = fmt.Sprintf("[%s] Error: notify the organizers and retry later", flag)
			resp.Status = "ERROR"
			return
		}
	} else {
		resp.Msg = fmt.Sprintf("[%s] Denied: flag already submitted", flag)
		resp.Status = "DENIED"
		log.Debugf("Flag %s from %s: already submitted", flag, team)
		return
	}

	// Calculate flag points in a db transaction to avoid inconsistencies on db
	scoreMutex.Lock()
	var offensePoints float64
	err = conn.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		attackerScore := new(db.ServiceScore)
		victimScore := new(db.ServiceScore)
		if err := conn.NewSelect().Model(attackerScore).Where("team = ? and service = ?", teamIP, info.Service).Scan(ctx); err != nil {
			return err
		}
		if err := conn.NewSelect().Model(victimScore).Where("team = ? and service = ?", info.Team, info.Service).Scan(ctx); err != nil {
			return err
		}
		offensePoints = scale / (1 + math.Exp((math.Sqrt(attackerScore.Score)-math.Sqrt(victimScore.Score))*norm))
		defensePoints := min(victimScore.Score, offensePoints)

		_, err = conn.NewInsert().Model(&db.FlagSubmission{
			FlagID:          info.ID,
			Team:            teamIP,
			OffensivePoints: offensePoints,
			DefensivePoints: defensePoints,
		}).Exec(ctx)
		if err != nil {
			return err
		}
		if _, err := conn.NewUpdate().Model(attackerScore).WherePK().Set("score = score + ?", offensePoints).Set("offense = offense + ?", offensePoints).Exec(ctx); err != nil {
			return err
		}
		if _, err := conn.NewUpdate().Model(victimScore).WherePK().Set("score = score - ?", defensePoints).Set("defense = defense - ?", defensePoints).Exec(ctx); err != nil {
			return err
		}

		return nil
	})
	scoreMutex.Unlock()

	if err != nil {
		resp.Msg = fmt.Sprintf("[%s] Error: notify the organizers and retry later", flag)
		resp.Status = "ERROR"
		log.Errorf("Error submitting flag: %v", err)
		return
	}

	resp.Status = "ACCEPTED"
	resp.Msg = fmt.Sprintf("[%s] Accepted: %f flag points", flag, offensePoints)
	log.Debugf("Flag %s from %s: %.02f flag points", flag, team, offensePoints)
}

func elaborateFlags(team *TeamInfo, submittedFlags []string, round uint) []SubResp {
	responses := make([]SubResp, 0, len(submittedFlags))
	for _, flag := range submittedFlags {
		resp := SubResp{
			Flag:   flag,
			Status: "RESUBMIT", // Default status
			Msg:    fmt.Sprintf("[%s] Unexpected Error, retry to send later", flag),
		}
		elaborateFlag(team, flag, &resp, round)
		responses = append(responses, resp)
	}
	return responses
}

func submitFlags(w http.ResponseWriter, r *http.Request) {

	if conf.GameEndTime != nil && time.Now().After(*conf.GameEndTime) {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	teamToken := r.Header.Get("X-Team-Token")
	currentTick := db.GetExposedRound()

	if currentTick < 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	teamInfo := conf.getTeamByToken(teamToken)
	if teamInfo == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if teamInfo.Nop {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	team := teamIDToIP(teamInfo.ID)
	// Checking if team lock exists, if not create it
	lockMappingMutex.Lock()
	if lockMap[team] == nil {
		lockMap[team] = new(sync.RWMutex)
	}
	lockMappingMutex.Unlock()
	// Locking the team avoiding multiple submission at the same time
	lockMap[team].Lock()
	defer lockMap[team].Unlock()

	if conf.SubmitterTimeout != nil {
		//Get last time
		lastSubmitTime, ok := lastSubmissionTime[team]
		if ok {
			//Check if the time has passed
			if time.Since(lastSubmitTime) < conf.SubmitterLimitTime {
				log.Infof("Submission limit reached for team %s", team)
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
		}
		lastSubmissionTime[team] = time.Now()
	}

	var submittedFlags []string
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&submittedFlags); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	submittedFlags = submittedFlags[:min(len(submittedFlags), conf.MaxFlagsPerRequest)]
	responses := elaborateFlags(teamInfo, submittedFlags, uint(currentTick))

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	if err := enc.Encode(responses); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func serveSubmission() {
	router := mux.NewRouter()
	router.HandleFunc("/flags", submitFlags).Methods("PUT")

	log.Noticef("Starting flag_submission on :8080")
	srv := &http.Server{
		Handler:      router,
		Addr:         "0.0.0.0:8080",
		WriteTimeout: 30 * time.Second,
		ReadTimeout:  30 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}
