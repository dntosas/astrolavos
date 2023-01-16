#!/usr/bin/env python3

# This script  is responsible for creating the release tags we use in our repos
# Based on the version we have in the version file, it's basically creating
# the tags locally and pushes them to upstream for each tag. It offers
# ability to release to multipe stacks using options.

import yaml
import sys
import re
import os.path
import argparse
import itertools
import subprocess


VALID_STACKS = [
    "pe",
    "cl",
    "co",
    "mx",
    "gr",
    "ar",
    "mgmt",
    "ctl",
    "dev",
    "sts",
    "tst",
    "glb"]
# regexp to match tags of format: v0.0.1rc123-dev
RE_TAG = re.compile(r"^v\d+\.\d+\.\d+(rc)(\d+?)?-.+?$")

# regexp to match production tags
RE_REL_TAG = re.compile(r"^v(\d+\.\d+\.\d+)-*.*?$")


class Releaser(object):
    """Class to hold the logic for releasing a new version of the repo"""

    def __init__(self, **kwargs):
        self.rc = ""
        self.skip_bump = False
        self.stacks = []

        if kwargs.get("stack", None) is not None:
            self.stacks = [kwargs["stack"]]

        if kwargs.get("all"):
            self.stacks = ["internal", "prod"]

        if kwargs.get("dev"):
            self.stacks = ["dev", "sts", "tst"]

        if kwargs.get("int"):
            self.stacks = ["internal"]

        if kwargs.get("prod"):
            self.stacks = ["prod"]

        self.major = kwargs.get("major")
        self.minor = kwargs.get("minor")
        self.patch = kwargs.get("patch")

        if not any([self.major, self.minor, self.patch]):
            self.skip_bump = True

        self.version = self.get_version()

        if kwargs.get("rc"):
            self.rc = self.get_rc_version()

    def release(self):
        """Releases to all specified stacks"""
        for stack in self.stacks:
            self.create_n_push_tag(stack)

    def create_n_push_tag(self, stack):
        """Creates locally the tag and pushes it to the upstream"""

        tag_name = self.get_tag_name(stack)

        print("Creating tag:{} locally".format(tag_name))
        try:
            subprocess.run(
                ["git", "tag", "-a", tag_name, "-m 'Release {}'".format(tag_name)],
                check=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT
            )
        except subprocess.CalledProcessError as e:
            # If git tag breaks because tag already exists continue with
            # the other tags if any
            if "already exists" in e.output.decode("utf-8"):
                print("Tag: {} already exists, skipping this one...".format(tag_name))
                return
            else:
                raise e

        print("Pushing tag:{} to upstream".format(tag_name))
        subprocess.run(
            ["git", "push", "origin", tag_name],
            check=True, stdout=subprocess.PIPE
        )

    def get_tag_name(self, stack):
        """
        Builds the tag name based on the stack, the version and if we
        want it to be a release candidate
        """

        if stack == "prod":
            # If prod, we specify a naked tag and Jenkinks knows that it has
            # to push to all prod stacks
            tag_name = "{}".format(self.version)
        else:
            tag_name = "{}{}-{}".format(self.version, self.rc, stack)

        return tag_name

    def get_version(self):
        """Get final version of our release"""

        new_version = self.get_new_version()

        if not new_version.startswith("v"):
            new_version = "v{}".format(new_version)

        print("New version: %s" % new_version)
        return new_version

    def get_new_version(self):
        """
        Find the new version that we will use considering
        user input, existence of VERSION file.
        """
        # If we find that a VERSION file exists then it should
        # get all precedence from any other options
        version_from_file = self.get_version_from_file()
        if version_from_file:
            return version_from_file

        # If we find that a helm charts file exists then it should
        # gets next precedence
        version_from_chart = self.get_chart_version()
        if version_from_chart:
            return version_from_chart

        current_version = self.get_previous_release_tag()
        # If user has selected to skip bumping of version
        # return the current one
        if self.skip_bump:
            return current_version

        # Split previous release and cast it to int to help further comparisons
        major, minor, patch = [int(x) for x in current_version.split(".")]

        new_version = ""
        if self.major:
            new_version = "{}.{}.{}".format(major + 1, 0, 0)
        elif self.minor:
            new_version = "{}.{}.{}".format(major, minor + 1, 0)
        elif self.patch:
            new_version = "{}.{}.{}".format(major, minor, patch + 1)

        return new_version

    def get_version_from_file(self):
        """Get version from reading the VERSION file in the root dir"""
        if not os.path.isfile("VERSION"):
            return None

        with open("VERSION", "r") as fd:
            return fd.read().strip("\n")

    def get_rc_version(self):
        """Get the next release candidate version based on the existing rc tags"""
        rc_tags = []

        all_tags = self.get_all_tags()

        for tag in all_tags:
            rc_match = RE_TAG.match(tag)
            # Get tags that belong to the current version and the are rc tags
            if (tag.startswith(self.version) and rc_match and rc_match.groups()[1] is not None):
                rc_tags.append(int(rc_match.groups()[1]))

        # If list is empty we don't have yet an rc tag
        if not rc_tags:
            return "rc1"
        else:
            # Get greater rc tag and return +1 version of it
            rc_tags.sort(reverse=True)
            last_tag = rc_tags[0]
            return "rc{}".format(int(last_tag) + 1)

    def get_previous_release_tag(self):
        """Get previous production tag or first commit"""

        rel_tags = []
        prev_rel_tag = "0.0.0"

        all_tags = self.get_all_tags()
        for tag in all_tags:
            # skip release candidate tags
            if "rc" in tag:
                continue
            rc_match = RE_REL_TAG.match(tag)
            if rc_match:
                rel_tags.append(rc_match.groups()[0])

        rel_tags.sort(reverse=True, key=lambda x: list(map(int, x.split('.'))))

        if len(rel_tags) > 0:
            prev_rel_tag = rel_tags[0]

        return prev_rel_tag

    def get_all_tags(self):
        """Get all local tags in a list"""
        res = subprocess.run(
            ["git", "tag"],
            check=True, stdout=subprocess.PIPE
        )
        return res.stdout.decode("utf-8").strip("\n").split("\n")

    def cleanup_tags(self):
        """Delete all rc tags for this version"""
        all_tags = self.get_all_tags()

        for tag in all_tags:
            if RE_TAG.match(tag):
                # Delete local tag
                print("Deleting tag: {} locally.".format(tag))
                subprocess.run(
                    ["git", "tag", "--delete", tag],
                    check=True, stdout=subprocess.PIPE
                )
                # Delete remote tag
                try:
                    print("Deleting tag: {} from upstream.".format(tag))
                    subprocess.run(
                        ["git", "push", "--delete", "origin", tag],
                        check=True, stdout=subprocess.PIPE,
                        stderr=subprocess.STDOUT
                    )
                except subprocess.CalledProcessError as e:
                    # If tag is only local, continue
                    if "remote ref does not exist" in e.output.decode("utf-8"):
                        print(
                            "Tag: {} doesn't exist in upstream, continueing...".format(tag))
                        continue
                    else:
                        raise e

    def get_chart_version(self):
        """Get the chart version from helm chart if it exists"""
        name = "Chart.yaml"
        chart_version = None

        for root, dirs, files in os.walk("charts/"):
            if name in files:
                with open(os.path.join(root, name), "r") as f:
                    try:
                        chart_version = yaml.safe_load(f)["version"]
                    except yaml.YAMLError as exc:
                        print(exc)
        return chart_version


def validate_input(args):
    """Run some validation checks on user's input"""

    # If cleanup_rc we don't care about other options
    if args.cleanup_rc:
        return

    if not any([args.stack, args.all, args.prod, args.int, args.dev]):
        print("You need to specify at least one of: stack/--all/--prod/--int/--dev.\nSee usage with ./release.py -h")
        sys.exit(-1)

    for c in itertools.combinations(
            [args.stack, args.all, args.prod, args.int, args.dev], 2):
        if all(c):
            print("You need to specify only one from: stack/--all/--prod/--int/--dev.\nSee usage with ./release.py -h")
            sys.exit(-1)

    if (args.rc and not args.stack) or (args.rc and any(
            [args.all, args.prod, args.int, args.dev])):
        print("When you specify release candidate option you have to specify also a specific stack.\nSee usage with ./release.py -h")
        sys.exit(-1)


def build_arguments():
    """Build the arguments of the cli tool"""

    def get_repo_name():
        repo_whole_name = subprocess.run(
            ["git", "rev-parse", "--show-toplevel"],
            check=True, stdout=subprocess.PIPE
        ).stdout.decode("utf-8")

        return os.path.basename(repo_whole_name).strip()

    repo_name = get_repo_name()

    description = """
    This program releases '{}' repo to a number of stacks based on the
    options/arguments you pass to it.

    e.g.
    > ./release.py pe # release current version in Peru stack
    > ./release.py sts --rc # deploy a release candidate tag to stresss
    > ./release.py --cleanup-rc # cleanup rc tags for current version
    > ./release.py --major --all # release the current version to all stacks by creating all individual tags
    > ./release.py --minor --dev # release the current version to dev stacks
    """.format(repo_name)

    parser = argparse.ArgumentParser(
        description=description,
        formatter_class=argparse.RawTextHelpFormatter)

    parser.add_argument(
        "stack",
        nargs='?',
        help="The stack the tool will use for releasing",
        type=str,
        choices=VALID_STACKS)
    parser.add_argument(
        "--all",
        help="Release to all stacks (pe, cl, co, mx, gr, ar, glb, mgmt, ctl, dev, sts, tst)",
        action="store_true")
    parser.add_argument(
        "--prod",
        help="Release to all production stacks (pe, cl, co, mx, gr, ar)",
        action="store_true")
    parser.add_argument(
        "--int",
        help="Release to all internal stacks (dev, sts, tst, ctl, mgmt)",
        action="store_true")
    parser.add_argument(
        "--dev",
        help="Release to all dev stacks (dev, sts, tst)",
        action="store_true")
    parser.add_argument(
        "--rc",
        help="Release to the specified stack as a release candidate (rc in the tag)",
        action="store_true")
    parser.add_argument(
        "--cleanup-rc",
        help="Clean up all release candidate tags that are associated with this version",
        action="store_true")

    version_constrains = parser.add_mutually_exclusive_group()
    version_constrains.add_argument(
        "--major",
        help="Bump major version",
        action="store_true")
    version_constrains.add_argument(
        "--minor",
        help="Bump minor version",
        action="store_true")
    version_constrains.add_argument(
        "--patch",
        help="Bump patch version",
        action="store_true")

    args = parser.parse_args()

    return args


def main():
    args = build_arguments()

    validate_input(args)

    releaser = Releaser(**{
        "stack": args.stack, "all": args.all, "prod": args.prod,
        "int": args.int, "dev": args.dev, "rc": args.rc,
        "major": args.major, "minor": args.minor, "patch": args.patch
    })

    if args.cleanup_rc:
        releaser.cleanup_tags()
        return

    releaser.release()


if __name__ == "__main__":
    sys.exit(main())
