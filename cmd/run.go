package cmd

import (
	"database/sql"
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

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use: "run",
	Run: func(cmd *cobra.Command, args []string) {
		usage := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "token"}, []string{"model"})
		users := prometheus.NewGauge(prometheus.GaugeOpts{Name: "user"})
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
			db, err := sql.Open("mysql", db)
			if err != nil {
				panic(err)
			}
			//nolint:errcheck
			defer db.Close()
			rows, err := db.Query("select tokentype, tokeninfo.key, count(*) from token left join tokeninfo on token.id = tokeninfo.token_id and tokeninfo.key='passkey' group by tokentype, tokeninfo.key")
			if err != nil {
				panic(err)
			}
			//nolint:errcheck
			defer rows.Close()
			for rows.Next() {
				var tokentype, key string
				var count int
				if err := rows.Scan(&tokentype, &key, &count); err != nil {
					panic(err)
				}
				if key == "passkey" {
					tokentype = key
				}
				usage.WithLabelValues(tokentype).Set(float64(count))
			}
			rows, err = db.Query("select count(distinct user_id) from tokenowner")
			if err != nil {
				panic(err)
			}
			//nolint:errcheck
			defer rows.Close()
			for rows.Next() {
				var count int
				if err := rows.Scan(&count); err != nil {
					panic(err)
				}
				users.Set(float64(count))
			}
		}))
		if err != nil {
			panic(err)
		}
		slog.Info("job created", slog.String("job", job.ID().String()))
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
