/**
 * Copyright (c) 2021 Gitpod GmbH. All rights reserved.
 * Licensed under the GNU Affero General Public License (AGPL).
 * See License-AGPL.txt in the project root for license information.
 */

import { MouseEvent, useState } from 'react';

export interface ContextMenuProps {
    children: React.ReactChild[] | React.ReactChild;
    menuEntries: ContextMenuEntry[];
    width?: string;
}

export interface ContextMenuEntry {
    title: string;
    active?: boolean;
    /**
     * whether a separator line should be rendered below this item
     */
    separator?: boolean;
    customFontStyle?: string;
    onClick?: (event: MouseEvent)=>void;
    href?: string;
}

function ContextMenu(props: ContextMenuProps) {
    const [expanded, setExpanded] = useState(false);
    const toggleExpanded = () => {
        setExpanded(!expanded);
    }

    if (expanded) {
        // HACK! I want to skip the bubbling phase of the current click
        setTimeout(() => {
            window.addEventListener('click', () => setExpanded(false), { once: true });
        }, 0);
    }
  
    const enhancedEntries = props.menuEntries.map(e => {
        return {
            ... e,
            onClick: (event: MouseEvent) => {
                e.onClick && e.onClick(event);
                toggleExpanded();
                event.preventDefault();
            }
        }
    })

    const font = "text-gray-600 hover:text-gray-800"

    const menuId = String(Math.random());

    return (
        <div className="relative cursor-pointer">
            <div onClick={(e) => {
                toggleExpanded();
                e.preventDefault();
            }}>
                {props.children}
            </div>
            {expanded?
                <div className={`mt-2 z-50 ${props.width || 'w-48'} bg-white absolute right-0 flex flex-col border border-gray-200 rounded-lg truncated`}>
                    {enhancedEntries.map((e, index) => {
                        const clickable = e.href || e.onClick;
                        const entry = <div key={e.title} className={`px-4 flex py-3 ${clickable?'hover:bg-gray-200':''} text-sm leading-1 ${e.customFontStyle || font} ${e.separator? ' border-b border-gray-200':''}`} >
                            <div>{e.title}</div><div className="flex-1"></div>{e.active ? <div className="pl-1 font-semibold">&#x2713;</div>: null}
                        </div>
                        if (!clickable) {
                            return entry;
                        }
                        return <a key={`entry-${menuId}-${index}-${e.title}`} href={e.href} onClick={e.onClick}>
                            {entry}
                        </a>
                    })}
                </div>
            :
                null
            }
        </div>
    );
}

export default ContextMenu;