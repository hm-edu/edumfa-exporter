package cmd

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-co-op/gocron/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"

	_ "github.com/go-sql-driver/mysql"
)

var db string
var usage *prometheus.GaugeVec
var users *prometheus.GaugeVec

func updateData() {
	db, err := sql.Open("mysql", db)
	if err != nil {
		slog.Error("failed creating connection query", slog.Any("error", err))
		return
	}
	//nolint:errcheck
	defer db.Close()
	rows, err := db.Query("select tokentype, IFNULL(rollout_state, ''), IFNULL(tokeninfo.key, ''), count(*) from token left join tokeninfo on token.id = tokeninfo.token_id and tokeninfo.key='passkey' group by tokentype, tokeninfo.key, rollout_state")
	if err != nil {
		slog.Error("failed executing query", slog.Any("error", err))
		return
	}
	//nolint:errcheck
	defer rows.Close()
	for rows.Next() {
		var tokentype, state, key string
		var count int
		if err := rows.Scan(&tokentype, &state, &key, &count); err != nil {
			slog.Error("failed parsing line", slog.Any("error", err))
			return
		}
		if key == "passkey" {
			tokentype = key
		}
		if state == "verify" || state == "clientwait" {
			tokentype = fmt.Sprintf("%s pending", tokentype)
		}
		usage.WithLabelValues(tokentype).Set(float64(count))
	}
	rowsOk, err := db.Query(
		`	select count(distinct user_id)
			from tokenowner 
			join token on token.id = tokenowner.token_id 
			where token.rollout_state NOT IN ('verify', 'clientwait') and user_id in (
			select user_id from tokenowner 
			join token on token.id = tokenowner.token_id 
			where token.rollout_state IN ('verify', 'clientwait'));`,
	)
	if err != nil {
		slog.Error("failed executing query", slog.Any("error", err))
		return
	}
	//nolint:errcheck
	defer rowsOk.Close()
	for rowsOk.Next() {
		var count int
		if err := rowsOk.Scan(&count); err != nil {
			slog.Error("failed parsing line", slog.Any("error", err))
			return
		}
		users.WithLabelValues("ok").Set(float64(count))
	}
	rowsIncomplete, err := db.Query(
		`	select count(distinct user_id)
			from tokenowner 
			join token on token.id = tokenowner.token_id 
			where token.rollout_state IN ('verify', 'clientwait') and user_id not in (
			select user_id from tokenowner 
			join token on token.id = tokenowner.token_id 
			where token.rollout_state NOT IN ('verify', 'clientwait'));`,
	)
	if err != nil {
		slog.Error("failed executing query", slog.Any("error", err))
		return
	}
	//nolint:errcheck
	defer rowsIncomplete.Close()
	for rowsIncomplete.Next() {
		var count int
		if err := rowsIncomplete.Scan(&count); err != nil {
			slog.Error("failed parsing line", slog.Any("error", err))
			return
		}
		users.WithLabelValues("incomplete").Set(float64(count))
	}
}

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use: "run",
	Run: func(cmd *cobra.Command, args []string) {
		usage = prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: "edumfa", Subsystem: "token", Name: "count"}, []string{"model"})
		users = prometheus.NewGaugeVec(prometheus.GaugeOpts{Namespace: "edumfa", Subsystem: "user", Name: "count"}, []string{"state"})
		prometheus.MustRegister(usage)
		prometheus.MustRegister(users)
		s, err := gocron.NewScheduler()
		if err != nil {
			panic(err)
		}

		s.Start()
		defer func(s gocron.Scheduler) {
			_ = s.Shutdown()
		}(s)

		job, err := s.NewJob(gocron.CronJob("*/2 * * * *", false), gocron.NewTask(func() {
			updateData()
		}))
		if err != nil {
			panic(err)
		}
		slog.Info("job created", slog.String("job", job.ID().String()))
		updateData()
		http.Handle("/metrics", promhttp.Handler())
		//nolint:errcheck
		http.ListenAndServe(":8080", nil)
		signalChan := make(chan os.Signal, 1)
		signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
		<-signalChan
		slog.Info("shutdown received")
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().StringVar(&db, "db", "", "database connection string")
}
