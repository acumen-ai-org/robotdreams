package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func newStorageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "storage",
		Short: "Read and write objects in the control plane's storage backend",
	}
	cmd.AddCommand(newStoragePutCmd(), newStorageGetCmd(), newStorageLsCmd())
	return cmd
}

type storageTargetFlags struct {
	Server   string
	ServerID string
	AsWorker string
}

func addStorageTargetFlags(cmd *cobra.Command, f *storageTargetFlags) {
	cmd.Flags().StringVar(&f.Server, "server", "", "control plane address (default: $"+envDreamURL+", else from local config)")
	cmd.Flags().StringVar(&f.ServerID, "server-id", "", "local identity directory under ~/.dream to use")
	cmd.Flags().StringVar(&f.AsWorker, "worker-id", "", "local worker identity to authenticate as (default: from local config)")
}

type storagePutOptions struct {
	storageTargetFlags

	Path string

	LocalFile string

	IfMatchRevision string
}

func newStoragePutCmd() *cobra.Command {
	var opts storagePutOptions

	cmd := &cobra.Command{
		Use:   "put <path> <local-file>",
		Short: "Upload a local file to an object path",
		Long: "Upload a local file to an object path. Use \"-\" as <local-file> to read\n" +
			"the object's bytes from stdin.\n\n" +
			"--if-match-revision makes the write conditional: it fails with a\n" +
			"conflict if the object has changed since that revision.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Path, opts.LocalFile = args[0], args[1]
			meta, err := runStoragePut(cmd.Context(), opts, cmd.InOrStdin())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (%d bytes, revision %s)\n", meta.Path, meta.Size, meta.Revision)
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.IfMatchRevision, "if-match-revision", "", "only write if this is still the object's current revision")
	addStorageTargetFlags(cmd, &opts.storageTargetFlags)

	return cmd
}

func runStoragePut(ctx context.Context, opts storagePutOptions, stdin io.Reader) (objectMetaView, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Path == "" {
		return objectMetaView{}, fmt.Errorf("object path is required")
	}

	var data []byte
	var err error
	if opts.LocalFile == "-" {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(opts.LocalFile)
	}
	if err != nil {
		return objectMetaView{}, fmt.Errorf("read %s: %w", opts.LocalFile, err)
	}

	target, err := resolveTarget(opts.Server, opts.ServerID, opts.AsWorker)
	if err != nil {
		return objectMetaView{}, err
	}
	client, err := target.oneShotClient(ctx)
	if err != nil {
		return objectMetaView{}, err
	}

	q := url.Values{"path": {opts.Path}}
	if opts.IfMatchRevision != "" {
		q.Set("if_match_revision", opts.IfMatchRevision)
	}

	resp, err := client.do(ctx, http.MethodPost, "/api/storage/objects?"+q.Encode(),
		"application/octet-stream", bytes.NewReader(data))
	if err != nil {
		return objectMetaView{}, err
	}
	defer resp.Body.Close()

	var meta objectMetaView
	if err := decodeJSONBody(resp.Body, &meta); err != nil {
		return objectMetaView{}, err
	}
	return meta, nil
}

type storageGetOptions struct {
	storageTargetFlags

	Path string

	LocalFile string
}

func newStorageGetCmd() *cobra.Command {
	var opts storageGetOptions

	cmd := &cobra.Command{
		Use:   "get <path> <local-file>",
		Short: "Download an object to a local file",
		Long: "Download an object to a local file. Use \"-\" as <local-file> to write\n" +
			"the bytes to stdout; the object's metadata always goes to stderr, so\n" +
			"stdout stays clean for piping.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Path, opts.LocalFile = args[0], args[1]
			return runStorageGet(cmd.Context(), opts, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}

	addStorageTargetFlags(cmd, &opts.storageTargetFlags)

	return cmd
}

func runStorageGet(ctx context.Context, opts storageGetOptions, stdout, meta io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Path == "" {
		return fmt.Errorf("object path is required")
	}

	target, err := resolveTarget(opts.Server, opts.ServerID, opts.AsWorker)
	if err != nil {
		return err
	}
	client, err := target.oneShotClient(ctx)
	if err != nil {
		return err
	}

	q := url.Values{"path": {opts.Path}}
	resp, err := client.do(ctx, http.MethodGet, "/api/storage/objects?"+q.Encode(), "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var dst io.Writer
	if opts.LocalFile == "-" {
		dst = stdout
	} else {
		f, err := os.Create(opts.LocalFile)
		if err != nil {
			return fmt.Errorf("create %s: %w", opts.LocalFile, err)
		}
		defer f.Close()
		dst = f
	}

	n, err := io.Copy(dst, resp.Body)
	if err != nil {
		return fmt.Errorf("write %s: %w", opts.LocalFile, err)
	}

	fmt.Fprintf(meta, "path=%s revision=%s size=%d updated_at=%s updated_by=%s\n",
		resp.Header.Get("X-Path"), resp.Header.Get("X-Revision"), n,
		orDash(resp.Header.Get("X-Updated-At")), orDash(resp.Header.Get("X-Updated-By")))
	return nil
}

type storageLsOptions struct {
	storageTargetFlags

	Prefix string
}

func newStorageLsCmd() *cobra.Command {
	var opts storageLsOptions

	cmd := &cobra.Command{
		Use:   "ls [prefix]",
		Short: "List objects under a prefix",
		Long:  "List objects under a prefix. With no prefix, lists every object.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				opts.Prefix = args[0]
			}
			objects, err := runStorageLs(cmd.Context(), opts)
			if err != nil {
				return err
			}
			return printObjectTable(cmd.OutOrStdout(), objects)
		},
	}

	addStorageTargetFlags(cmd, &opts.storageTargetFlags)

	return cmd
}

func runStorageLs(ctx context.Context, opts storageLsOptions) ([]objectMetaView, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	target, err := resolveTarget(opts.Server, opts.ServerID, opts.AsWorker)
	if err != nil {
		return nil, err
	}
	client, err := target.oneShotClient(ctx)
	if err != nil {
		return nil, err
	}

	var out struct {
		Objects []objectMetaView `json:"objects"`
	}
	if err := client.doJSON(ctx, http.MethodGet, storageLsPath(opts.Prefix), nil, &out); err != nil {
		return nil, err
	}
	return out.Objects, nil
}

func storageLsPath(prefix string) string {
	return "/api/storage/objects?" + url.Values{"prefix": {prefix}}.Encode()
}

func printObjectTable(out io.Writer, objects []objectMetaView) error {
	if len(objects) == 0 {
		fmt.Fprintln(out, "no objects")
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PATH\tSIZE\tREVISION\tUPDATED BY\tUPDATED AT")
	for _, o := range objects {
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\n",
			o.Path, o.Size, orDash(o.Revision), orDash(o.UpdatedBy), formatTime(o.UpdatedAt))
	}
	return tw.Flush()
}
