/**
 * Copyright (c) 2021 Gitpod GmbH. All rights reserved.
 * Licensed under the GNU Affero General Public License (AGPL).
 * See License-AGPL.txt in the project root for license information.
 */

import { User } from "@gitpod/gitpod-protocol";
import { useContext } from "react";
import { Link } from "react-router-dom";
import { gitpodHostUrl } from "../service/service";
import { UserContext } from "../user-context";
import ContextMenu from "./ContextMenu";
import * as images from '../images';
interface Entry {
    title: string, link: string
}

function MenuItem(entry: Entry) {
    let classes = "flex block text-sm font-medium lg:px-3 px-0 py-1.5 rounded-md";
    if (window.location.pathname.toLowerCase() === entry.link.toLowerCase()) {
        classes += " bg-gray-200";
    } else {
        classes += " text-gray-600 hover:bg-gray-100 ";
    }
    return <li key={entry.title}>
        {entry.link.startsWith('https://')
            ? <a className={classes} href={entry.link}>
                <div>{entry.title}</div>
            </a>
            : <Link className={classes} to={entry.link}>
                <div>{entry.title}</div>
            </Link>}
    </li>;
}

function Menu(props: { left: Entry[], right: Entry[] }) {
    const { user } = useContext(UserContext);

    return (
        <header className="lg:px-28 px-10 bg-white flex flex-wrap items-center py-4">
            <style dangerouslySetInnerHTML={{
                __html: `
                #menu-toggle:checked+#menu {
                    display: block;
                }
                `}} />
            <div className="flex justify-between items-center pr-3">
                <Link to="/">
                    <img src={images.gitpodIcon} className="h-6" />
                </Link>
            </div>
            <div className="lg:hidden flex-grow" />
            <label htmlFor="menu-toggle" className="pointer-cursor lg:hidden block">
                <svg className="fill-current text-gray-700"
                    xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 20 20">
                    <title>menu</title>
                    <path d="M0 3h20v2H0V3zm0 6h20v2H0V9zm0 6h20v2H0v-2z"></path>
                </svg>
            </label>
            <input className="hidden" type="checkbox" id="menu-toggle" />
            <div className="hidden lg:flex lg:flex-1 lg:items-center lg:w-auto w-full" id="menu">
                <nav className="lg:flex-1">
                    <ul className="lg:flex lg:flex-1 items-center justify-between text-base text-gray-700 space-x-2">
                        {props.left.map(MenuItem)}
                        <li className="flex-1"></li>
                        {props.right.map(MenuItem)}
                    </ul>
                </nav>
                <div className="lg:ml-3 flex items-center justify-start lg:mb-0 mb-4 pointer-cursor m-l-auto rounded-full border-2 border-white hover:border-gray-200 p-0.5 font-medium">
                    <ContextMenu menuEntries={[
                    {
                        title: (user && User.getPrimaryEmail(user)) || '',
                        customFontStyle: 'text-gray-400',
                        separator: true
                    },
                    {
                        title: 'Settings',
                        href: '/settings',
                        separator: true
                    },
                    {
                        title: 'Logout',
                        href: gitpodHostUrl.asApiLogout().toString()
                    },
                    ]}>
                        <img className="rounded-full w-6 h-6"
                            src={user?.avatarUrl || ''} alt={user?.name || 'Anonymous'} />
                    </ContextMenu>
                </div>
            </div>
        </header>
    );
}

export default Menu;